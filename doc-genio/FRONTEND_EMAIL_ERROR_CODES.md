# 前端开发者：邮箱登录注册错误码

## 快速参考表

| 错误码 | 枚举名称 | 错误消息（英文） | 建议的中文提示 | 使用场景 |
|-------|---------|----------------|---------------|---------|
| **4017** | `INVALID_EMAIL_FORMAT` | "Invalid email format" | "请输入正确的邮箱格式" | 邮箱格式验证失败 |
| **4018** | `INVALID_EMAIL_OR_PASSWORD` | "Invalid email or password" | "邮箱或密码错误" | 邮箱密码登录失败 |
| **4019** | `EMAIL_ALREADY_EXISTS` | "Email already registered" | "该邮箱已被注册" | 注册时邮箱已存在 |
| **4020** | `EMAIL_NOT_REGISTERED` | "Email not registered" | "该邮箱未注册" | 邮箱未注册 |
| **4021** | `INVALID_PASSWORD_FORMAT` | "Password does not meet requirements" | "密码格式不符合要求" | 密码格式验证失败 |
| **4022** | `INVALID_EMAIL_VERIFY_CODE` | "Invalid verification code" | "验证码错误" | 验证码不正确 |
| **4023** | `EMAIL_VERIFY_CODE_EXPIRED` | "Verification code expired" | "验证码已过期" | 验证码已过期 |
| **4024** | `PASSWORD_NOT_SET` | "Please use verification code to login" | "账号未设置密码，请使用验证码登录" | 账号未设置密码 |

## TypeScript 类型定义

```typescript
/**
 * 邮箱认证错误码
 */
export enum EmailAuthErrorCode {
  /** 邮箱格式无效 */
  INVALID_EMAIL_FORMAT = 4017,
  /** 邮箱或密码错误 */
  INVALID_EMAIL_OR_PASSWORD = 4018,
  /** 邮箱已注册 */
  EMAIL_ALREADY_EXISTS = 4019,
  /** 邮箱未注册 */
  EMAIL_NOT_REGISTERED = 4020,
  /** 密码格式不符合要求 */
  INVALID_PASSWORD_FORMAT = 4021,
  /** 邮箱验证码错误 */
  INVALID_EMAIL_VERIFY_CODE = 4022,
  /** 邮箱验证码过期 */
  EMAIL_VERIFY_CODE_EXPIRED = 4023,
  /** 账号未设置密码 */
  PASSWORD_NOT_SET = 4024,
}

/**
 * 错误码配置
 */
export const EmailErrorConfig = {
  [EmailAuthErrorCode.INVALID_EMAIL_FORMAT]: {
    code: 4017,
    message: '请输入正确的邮箱格式',
    action: 'checkEmailFormat',
  },
  [EmailAuthErrorCode.INVALID_EMAIL_OR_PASSWORD]: {
    code: 4018,
    message: '邮箱或密码错误',
    action: 'retry',
  },
  [EmailAuthErrorCode.EMAIL_ALREADY_EXISTS]: {
    code: 4019,
    message: '该邮箱已被注册',
    action: 'navigateToLogin',
  },
  [EmailAuthErrorCode.EMAIL_NOT_REGISTERED]: {
    code: 4020,
    message: '该邮箱未注册',
    action: 'navigateToRegister',
  },
  [EmailAuthErrorCode.INVALID_PASSWORD_FORMAT]: {
    code: 4021,
    message: '密码格式不符合要求（6-20位字符）',
    action: 'checkPasswordFormat',
  },
  [EmailAuthErrorCode.INVALID_EMAIL_VERIFY_CODE]: {
    code: 4022,
    message: '验证码错误，请重新输入',
    action: 'clearCode',
  },
  [EmailAuthErrorCode.EMAIL_VERIFY_CODE_EXPIRED]: {
    code: 4023,
    message: '验证码已过期，请重新获取',
    action: 'resendCode',
  },
  [EmailAuthErrorCode.PASSWORD_NOT_SET]: {
    code: 4024,
    message: '账号未设置密码，请使用验证码登录',
    action: 'switchToCodeLogin',
  },
} as const;
```

## JavaScript 实现示例

### 1. 错误处理函数

```javascript
/**
 * 处理邮箱认证错误
 * @param {number} errorCode - 错误码
 * @param {string} errorMsg - 错误消息
 */
function handleEmailAuthError(errorCode, errorMsg) {
  switch (errorCode) {
    case 4017: // INVALID_EMAIL_FORMAT
      showError('请输入正确的邮箱格式');
      highlightField('email');
      break;

    case 4018: // INVALID_EMAIL_OR_PASSWORD
      showError('邮箱或密码错误，请重试');
      highlightFields(['email', 'password']);
      clearPasswordField();
      break;

    case 4019: // EMAIL_ALREADY_EXISTS
      showError('该邮箱已被注册，是否直接登录？', {
        confirmText: '去登录',
        onConfirm: () => navigateToLogin(),
      });
      break;

    case 4020: // EMAIL_NOT_REGISTERED
      showError('该邮箱未注册，是否立即注册？', {
        confirmText: '去注册',
        onConfirm: () => navigateToRegister(),
      });
      break;

    case 4021: // INVALID_PASSWORD_FORMAT
      showError('密码格式不符合要求\n密码长度需为6-20个字符');
      highlightField('password');
      showPasswordHint();
      break;

    case 4022: // INVALID_EMAIL_VERIFY_CODE
      showError('验证码错误，请重新输入');
      clearVerifyCode();
      highlightField('verifyCode');
      break;

    case 4023: // EMAIL_VERIFY_CODE_EXPIRED
      showError('验证码已过期，请重新获取', {
        confirmText: '重新获取',
        onConfirm: () => resendVerifyCode(),
      });
      enableResendButton();
      break;

    case 4024: // PASSWORD_NOT_SET
      showInfo('该账号未设置密码，已自动切换到验证码登录');
      switchToVerifyCodeMode();
      break;

    default:
      showError(errorMsg || '操作失败，请重试');
  }
}
```

### 2. React 组件示例

```jsx
import { useState } from 'react';
import { EmailAuthErrorCode } from './constants';

function EmailLoginForm() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');

  const handleLogin = async () => {
    try {
      const response = await api.emailPasswordAuth({
        email,
        password,
      });

      if (response.response_header.code !== 0) {
        handleError(response.response_header);
        return;
      }

      // 登录成功
      onLoginSuccess(response);
    } catch (err) {
      setError('网络错误，请稍后重试');
    }
  };

  const handleError = (header) => {
    const code = header.response_status_code;

    switch (code) {
      case EmailAuthErrorCode.INVALID_EMAIL_FORMAT:
        setError('请输入正确的邮箱格式');
        break;

      case EmailAuthErrorCode.INVALID_EMAIL_OR_PASSWORD:
        setError('邮箱或密码错误');
        setPassword(''); // 清空密码
        break;

      case EmailAuthErrorCode.EMAIL_NOT_REGISTERED:
        setError('该邮箱未注册');
        // 显示注册引导
        showRegisterPrompt();
        break;

      case EmailAuthErrorCode.PASSWORD_NOT_SET:
        setError('账号未设置密码，请使用验证码登录');
        // 自动切换到验证码登录
        switchToCodeMode();
        break;

      default:
        setError(header.msg || '登录失败，请重试');
    }
  };

  return (
    <div className="login-form">
      <input
        type="email"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        placeholder="请输入邮箱"
      />
      <input
        type="password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
        placeholder="请输入密码"
      />
      {error && <div className="error-message">{error}</div>}
      <button onClick={handleLogin}>登录</button>
    </div>
  );
}
```

### 3. Vue 组件示例

```vue
<template>
  <div class="email-register-form">
    <el-input
      v-model="email"
      placeholder="请输入邮箱"
      :class="{ error: errors.email }"
    />
    <el-input
      v-model="verifyCode"
      placeholder="请输入验证码"
      :class="{ error: errors.verifyCode }"
    >
      <template #append>
        <el-button
          :disabled="countdown > 0"
          @click="sendVerifyCode"
        >
          {{ countdown > 0 ? `${countdown}秒后重试` : '获取验证码' }}
        </el-button>
      </template>
    </el-input>
    <el-input
      v-model="password"
      type="password"
      placeholder="请输入密码(6-20位)"
      :class="{ error: errors.password }"
    />
    <div v-if="errorMessage" class="error-tip">{{ errorMessage }}</div>
    <el-button type="primary" @click="handleRegister">注册</el-button>
  </div>
</template>

<script setup>
import { ref } from 'vue';
import { EmailAuthErrorCode } from './constants';

const email = ref('');
const verifyCode = ref('');
const password = ref('');
const errorMessage = ref('');
const errors = ref({});
const countdown = ref(0);

const handleRegister = async () => {
  try {
    const response = await api.emailRegister({
      email: email.value,
      verifyCode: verifyCode.value,
      password: password.value,
      confirmPassword: password.value,
    });

    if (response.response_header.code !== 0) {
      handleError(response.response_header);
      return;
    }

    // 注册成功
    ElMessage.success('注册成功！');
    router.push('/home');
  } catch (err) {
    ElMessage.error('网络错误，请稍后重试');
  }
};

const handleError = (header) => {
  const code = header.response_status_code;
  errors.value = {};

  switch (code) {
    case EmailAuthErrorCode.INVALID_EMAIL_FORMAT:
      errorMessage.value = '请输入正确的邮箱格式';
      errors.value.email = true;
      break;

    case EmailAuthErrorCode.EMAIL_ALREADY_EXISTS:
      errorMessage.value = '该邮箱已被注册';
      errors.value.email = true;
      // 显示跳转到登录的提示
      ElMessageBox.confirm('该邮箱已注册，是否前往登录？', '提示', {
        confirmButtonText: '去登录',
        cancelButtonText: '取消',
      }).then(() => {
        router.push('/login');
      });
      break;

    case EmailAuthErrorCode.INVALID_PASSWORD_FORMAT:
      errorMessage.value = '密码格式不符合要求（6-20位字符）';
      errors.value.password = true;
      break;

    case EmailAuthErrorCode.INVALID_EMAIL_VERIFY_CODE:
      errorMessage.value = '验证码错误，请重新输入';
      errors.value.verifyCode = true;
      verifyCode.value = '';
      break;

    case EmailAuthErrorCode.EMAIL_VERIFY_CODE_EXPIRED:
      errorMessage.value = '验证码已过期，请重新获取';
      errors.value.verifyCode = true;
      countdown.value = 0; // 允许重新发送
      break;

    default:
      errorMessage.value = header.msg || '操作失败，请重试';
  }
};

const sendVerifyCode = async () => {
  // 发送验证码逻辑
  // ...
};
</script>
```

## 用户提示文案建议

### 简洁版（适合 Toast/Message）

```javascript
const ErrorMessages = {
  4017: '邮箱格式错误',
  4018: '邮箱或密码错误',
  4019: '邮箱已被注册',
  4020: '邮箱未注册',
  4021: '密码格式错误',
  4022: '验证码错误',
  4023: '验证码已过期',
  4024: '请使用验证码登录',
};
```

### 详细版（适合 Dialog/Modal）

```javascript
const DetailedErrorMessages = {
  4017: {
    title: '邮箱格式错误',
    message: '请输入正确的邮箱地址，例如：example@email.com',
  },
  4018: {
    title: '登录失败',
    message: '邮箱或密码错误，请检查后重试',
  },
  4019: {
    title: '邮箱已注册',
    message: '该邮箱已被注册，您可以直接登录',
    action: { text: '去登录', link: '/login' },
  },
  4020: {
    title: '邮箱未注册',
    message: '该邮箱尚未注册，请先完成注册',
    action: { text: '去注册', link: '/register' },
  },
  4021: {
    title: '密码格式错误',
    message: '密码长度需为6-20个字符',
  },
  4022: {
    title: '验证码错误',
    message: '您输入的验证码不正确，请重新输入',
  },
  4023: {
    title: '验证码已过期',
    message: '验证码有效期为5分钟，请重新获取',
    action: { text: '重新获取', callback: 'resendCode' },
  },
  4024: {
    title: '提示',
    message: '该账号未设置密码，请使用验证码登录',
    action: { text: '使用验证码', callback: 'switchMode' },
  },
};
```

## 接口响应示例

### 成功响应
```json
{
  "response_header": {
    "req_id": "xxx",
    "code": 0,
    "msg": "success",
    "response_status_code": 200
  },
  "access_token": "eyJhbGc...",
  "user_info": { ... }
}
```

### 错误响应示例

#### 邮箱格式错误 (4017)
```json
{
  "response_header": {
    "req_id": "xxx",
    "code": 4017,
    "msg": "Invalid email format",
    "response_status_code": 4017
  }
}
```

#### 邮箱或密码错误 (4018)
```json
{
  "response_header": {
    "req_id": "xxx",
    "code": 4018,
    "msg": "Invalid email or password",
    "response_status_code": 4018
  }
}
```

#### 邮箱已注册 (4019)
```json
{
  "response_header": {
    "req_id": "xxx",
    "code": 4019,
    "msg": "Email already registered",
    "response_status_code": 4019
  }
}
```

#### 验证码错误 (4022)
```json
{
  "response_header": {
    "req_id": "xxx",
    "code": 4022,
    "msg": "Invalid verification code",
    "response_status_code": 4022
  }
}
```

## 最佳实践

### 1. 统一错误处理

```typescript
// 创建一个统一的错误处理器
class EmailAuthErrorHandler {
  private errorHandlers: Map<number, (msg: string) => void>;

  constructor() {
    this.errorHandlers = new Map([
      [4017, this.handleInvalidEmailFormat],
      [4018, this.handleInvalidCredentials],
      [4019, this.handleEmailExists],
      [4020, this.handleEmailNotRegistered],
      [4021, this.handleInvalidPasswordFormat],
      [4022, this.handleInvalidCode],
      [4023, this.handleCodeExpired],
      [4024, this.handlePasswordNotSet],
    ]);
  }

  handle(errorCode: number, errorMsg: string) {
    const handler = this.errorHandlers.get(errorCode);
    if (handler) {
      handler.call(this, errorMsg);
    } else {
      this.handleUnknownError(errorMsg);
    }
  }

  private handleInvalidEmailFormat(msg: string) {
    Toast.show('请输入正确的邮箱格式');
  }

  private handleInvalidCredentials(msg: string) {
    Toast.show('邮箱或密码错误');
    // 清空密码字段
    FormStore.clearPassword();
  }

  private handleEmailExists(msg: string) {
    Dialog.confirm({
      title: '邮箱已注册',
      message: '该邮箱已被注册，是否前往登录？',
      confirmText: '去登录',
      onConfirm: () => Router.push('/login'),
    });
  }

  private handleEmailNotRegistered(msg: string) {
    Dialog.confirm({
      title: '邮箱未注册',
      message: '该邮箱尚未注册，是否立即注册？',
      confirmText: '去注册',
      onConfirm: () => Router.push('/register'),
    });
  }

  // ... 其他处理方法
}

// 使用
const errorHandler = new EmailAuthErrorHandler();
errorHandler.handle(response.response_header.response_status_code, response.response_header.msg);
```

### 2. 表单验证

```typescript
// 前端表单验证（在发送请求前）
const validateEmailForm = (email: string, password: string) => {
  // 邮箱格式验证
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  if (!emailRegex.test(email)) {
    throw new Error('请输入正确的邮箱格式');
  }

  // 密码格式验证
  if (password.length < 6 || password.length > 20) {
    throw new Error('密码长度需为6-20个字符');
  }

  return true;
};
```

### 3. 用户体验优化

```javascript
// 自动重试验证码
async function handleExpiredCode() {
  const result = await Dialog.confirm({
    title: '验证码已过期',
    message: '是否重新获取验证码？',
    confirmText: '重新获取',
  });

  if (result) {
    await sendVerifyCode();
    Toast.show('验证码已发送');
  }
}

// 智能切换登录方式
function handlePasswordNotSet() {
  Toast.show('该账号未设置密码，已切换到验证码登录');
  // 自动切换到验证码登录模式
  setLoginMode('verifyCode');
  // 自动发送验证码
  sendVerifyCode();
}
```

## 注意事项

1. **错误码优先级**
   - 优先检查 `response_status_code` 字段
   - 如果不存在，再检查 `code` 字段
   - 两者数值应该相同

2. **用户体验**
   - 对于 `4018` (邮箱或密码错误)，不要区分是邮箱还是密码错误，防止恶意探测
   - 对于 `4023` (验证码过期)，自动提供重新获取的入口
   - 对于 `4024` (密码未设置)，自动切换到验证码登录模式

3. **安全性**
   - 不要在前端显示过于详细的错误信息
   - 限制重试次数
   - 验证码输入错误多次后增加人机验证

4. **国际化**
   ```typescript
   const i18nErrorMessages = {
     'zh-CN': {
       4017: '请输入正确的邮箱格式',
       4018: '邮箱或密码错误',
       // ...
     },
     'en-US': {
       4017: 'Invalid email format',
       4018: 'Invalid email or password',
       // ...
     },
   };
   ```

## 相关接口

### 1. 发送验证码
- **接口**: `SendEmailVerifyCode`
- **可能的错误码**: `4017`

### 2. 邮箱验证码登录
- **接口**: `EmailAuth`
- **可能的错误码**: `4020`, `4022`, `4023`

### 3. 邮箱密码登录
- **接口**: `EmailPasswordAuth`
- **可能的错误码**: `4018`, `4020`, `4024`

### 4. 邮箱注册
- **接口**: `EmailRegister`
- **可能的错误码**: `4017`, `4019`, `4021`, `4022`, `4023`

## 联系方式

如有疑问，请联系：
- 后端负责人：[backend-team@example.com]
- API 文档：[https://api-docs.example.com]

---

**更新时间**: 2025-01-29
**版本**: v1.0
**维护者**: VisionAI Backend Team
