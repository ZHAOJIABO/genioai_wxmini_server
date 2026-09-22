// GenioAI Admin 后台管理应用
function adminApp() {
    return {
        // 状态
        token: localStorage.getItem('admin_token') || '',
        adminUser: localStorage.getItem('admin_user') || '',
        page: 'dashboard',
        loading: false,
        error: '',
        message: '',
        sidebarOpen: false,

        // 登录表单
        loginForm: { username: '', password: '' },

        // 仪表盘
        dashboard: null,

        // 用户管理
        users: [],
        userTotal: 0,
        userPage: 1,
        userSize: 20,
        userSearch: '',

        // 积分管理
        creditForm: { email: '', amount: 0, description: '' },
        creditQueryUID: '',
        creditBalance: null,

        // 统计
        dailyTasks: [],
        userStats: [],

        // 配置
        configs: [],

        // 审计日志
        auditLogs: [],
        auditTotal: 0,
        auditPage: 1,
        auditSize: 20,

        // 初始化
        init() {
            if (this.token) {
                this.loadDashboard();
            }
            // 处理 hash 路由
            window.addEventListener('hashchange', () => this.handleHash());
            this.handleHash();
        },

        handleHash() {
            const hash = window.location.hash.replace('#/', '') || 'dashboard';
            this.navigate(hash);
        },

        // 导航
        navigate(p) {
            this.page = p;
            window.location.hash = '#/' + p;
            switch(p) {
                case 'dashboard': this.loadDashboard(); break;
                case 'users': this.loadUsers(); break;
                case 'credits': break;
                case 'stats': this.loadStats(); break;
                case 'configs': this.loadConfigs(); break;
                case 'audit': this.loadAuditLogs(); break;
            }
        },

        // API 请求
        async api(path, options = {}) {
            const url = '/admin/api' + path;
            const headers = { 'Content-Type': 'application/json' };
            if (this.token) {
                headers['Authorization'] = 'Bearer ' + this.token;
            }
            try {
                const resp = await fetch(url, { ...options, headers });
                if (resp.status === 401) {
                    this.logout();
                    return null;
                }
                const data = await resp.json();
                if (data.code !== 0 && resp.status !== 200) {
                    this.showMessage(data.msg || '请求失败');
                    return null;
                }
                return data;
            } catch (e) {
                this.showMessage('网络请求失败');
                return null;
            }
        },

        // 登录
        async login() {
            this.loading = true;
            this.error = '';
            try {
                const resp = await fetch('/admin/api/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(this.loginForm)
                });
                const data = await resp.json();
                if (data.code === 0 && data.data) {
                    this.token = data.data.token;
                    this.adminUser = data.data.display_name || data.data.username;
                    localStorage.setItem('admin_token', this.token);
                    localStorage.setItem('admin_user', this.adminUser);
                    this.loadDashboard();
                } else {
                    this.error = data.msg || '登录失败';
                }
            } catch (e) {
                this.error = '网络请求失败';
            }
            this.loading = false;
        },

        // 登出
        logout() {
            this.token = '';
            this.adminUser = '';
            localStorage.removeItem('admin_token');
            localStorage.removeItem('admin_user');
        },

        // 仪表盘
        async loadDashboard() {
            const data = await this.api('/dashboard');
            if (data) this.dashboard = data.data;
        },

        // 用户管理
        async loadUsers() {
            const params = `?page=${this.userPage}&size=${this.userSize}&search=${encodeURIComponent(this.userSearch)}`;
            const data = await this.api('/users' + params);
            if (data && data.data) {
                this.users = data.data.list || [];
                this.userTotal = data.data.total || 0;
            }
        },

        async banUser(userID) {
            if (!confirm('确定封禁该用户？')) return;
            await this.api('/users/' + userID + '/ban', { method: 'POST' });
            this.showMessage('用户已封禁');
            this.loadUsers();
        },

        async unbanUser(userID) {
            if (!confirm('确定解封该用户？')) return;
            await this.api('/users/' + userID + '/unban', { method: 'POST' });
            this.showMessage('用户已解封');
            this.loadUsers();
        },

        // 积分
        async addCredits() {
            if (!this.creditForm.email || !this.creditForm.amount) {
                this.showMessage('请填写邮箱和积分数量');
                return;
            }
            this.loading = true;
            const data = await this.api('/credits/add', {
                method: 'POST',
                body: JSON.stringify(this.creditForm)
            });
            if (data && data.data) {
                this.showMessage('添加成功！已添加 ' + this.creditForm.amount + ' 积分');
                this.creditForm = { email: '', amount: 0, description: '' };
            }
            this.loading = false;
        },

        async queryBalance() {
            if (!this.creditQueryUID) {
                this.showMessage('请输入用户ID');
                return;
            }
            const data = await this.api('/credits/' + this.creditQueryUID);
            if (data) this.creditBalance = data.data;
        },

        // 统计
        async loadStats() {
            const daily = await this.api('/stats/tasks/daily?days=7');
            if (daily && daily.data) this.dailyTasks = daily.data;

            const users = await this.api('/stats/users?page=1&size=20');
            if (users && users.data) this.userStats = users.data.list || [];
        },

        // 配置
        async loadConfigs() {
            const data = await this.api('/configs');
            if (data && data.data) this.configs = data.data;
        },

        async updateConfig(key, value) {
            await this.api('/configs/' + key, {
                method: 'PUT',
                body: JSON.stringify({ value })
            });
            this.showMessage('配置已更新');
        },

        // 审计日志
        async loadAuditLogs() {
            const data = await this.api('/audit-logs?page=' + this.auditPage + '&size=' + this.auditSize);
            if (data && data.data) {
                this.auditLogs = data.data.list || [];
                this.auditTotal = data.data.total || 0;
            }
        },

        // 工具函数
        formatTime(ts) {
            if (!ts) return '-';
            const d = new Date(ts * 1000);
            return d.toLocaleDateString() + ' ' + d.toLocaleTimeString();
        },

        formatGormTime(t) {
            if (!t) return '-';
            return new Date(t).toLocaleString();
        },

        showMessage(msg) {
            this.message = msg;
            setTimeout(() => { this.message = ''; }, 3000);
        }
    };
}
