package task

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"va_visionai_server/internal/model"
	"va_visionai_server/internal/quota"
	reconciler "va_visionai_server/internal/task/reconciler"
	slot "va_visionai_server/internal/task/slot"
	pb "va_visionai_server/internal/va_interface"
)

// MockQueueSlotReleaser 用于测试的 Mock
type MockQueueSlotReleaser struct {
	mock.Mock
}

func (m *MockQueueSlotReleaser) ReleaseQueueSlot(ctx context.Context, userID, taskID string) error {
	args := m.Called(ctx, userID, taskID)
	return args.Error(0)
}

func (m *MockQueueSlotReleaser) IsDegradeMode() bool {
	args := m.Called()
	return args.Bool(0)
}

// MockTaskDao 用于测试的 Mock
type MockTaskDao struct {
	mock.Mock
}

func (m *MockTaskDao) GetTask(ctx context.Context, taskID string) (*model.PictureTask, error) {
	args := m.Called(ctx, taskID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.PictureTask), args.Error(1)
}

// TestParseUserIDFromKey_Queue 测试通用解析函数在队列前后缀下的行为
func TestParseUserIDFromKey_Queue(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
		ok       bool
	}{
		{
			name:     "valid queue key",
			key:      "visionai:queue:user123:tasks",
			expected: "user123",
			ok:       true,
		},
		{
			name:     "invalid prefix",
			key:      "visionai:concurrent:user123:tasks",
			expected: "",
			ok:       false,
		},
		{
			name:     "invalid suffix",
			key:      "visionai:queue:user123:items",
			expected: "",
			ok:       false,
		},
		{
			name:     "empty user id",
			key:      "visionai:queue::tasks",
			expected: "",
			ok:       false,
		},
		{
			name:     "no user id",
			key:      "visionai:queue:tasks",
			expected: "",
			ok:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID, ok := reconciler.ParseUserIDFromKey(tt.key, quota.QueueTasksKeyPrefix, quota.TasksKeySuffix)
			assert.Equal(t, tt.expected, userID)
			assert.Equal(t, tt.ok, ok)
		})
	}
}

// TestIsActiveStatus 测试活跃状态判断
func TestIsActiveStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   int32
		expected bool
	}{
		{
			name:     "PENDING is active",
			status:   int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING),
			expected: true,
		},
		{
			name:     "PROCESSING is active",
			status:   int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING),
			expected: true,
		},
		{
			name:     "PENDING_SUBMISSION_RETRY is active",
			status:   int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING_SUBMISSION_RETRY),
			expected: true,
		},
		{
			name:     "AWAITING_PROVIDER_COMPLETION is active",
			status:   int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_AWAITING_PROVIDER_COMPLETION),
			expected: true,
		},
		{
			name:     "COMPLETED is not active",
			status:   int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED),
			expected: false,
		},
		{
			name:     "FAILED is not active",
			status:   int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 使用适配器判定活跃态，避免重复函数
			adapter := &reconciler.QueueAdapter{}
			result := adapter.IsActive(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestQueueSlotReconciler_ProcessOne 测试单个任务处理逻辑
func TestQueueSlotReconciler_ProcessOne(t *testing.T) {
	// 启动 miniredis
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	// 创建 Redis 客户端
	rdb := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	tests := []struct {
		name          string
		taskID        string
		userID        string
		task          *model.PictureTask
		taskErr       error
		expectRelease bool
		degradeMode   bool
	}{
		{
			name:          "task not found - should release",
			taskID:        "task1",
			userID:        "user1",
			task:          nil,
			taskErr:       nil,
			expectRelease: true,
			degradeMode:   false,
		},
		{
			name:   "completed task - should release",
			taskID: "task2",
			userID: "user2",
			task: &model.PictureTask{
				TaskID: "task2",
				UserID: "user2",
				Status: int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_COMPLETED),
			},
			taskErr:       nil,
			expectRelease: true,
			degradeMode:   false,
		},
		{
			name:   "failed task - should release",
			taskID: "task3",
			userID: "user3",
			task: &model.PictureTask{
				TaskID: "task3",
				UserID: "user3",
				Status: int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_FAILED),
			},
			taskErr:       nil,
			expectRelease: true,
			degradeMode:   false,
		},
		{
			name:   "pending task - should not release",
			taskID: "task4",
			userID: "user4",
			task: &model.PictureTask{
				TaskID: "task4",
				UserID: "user4",
				Status: int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PENDING),
			},
			taskErr:       nil,
			expectRelease: false,
			degradeMode:   false,
		},
		{
			name:   "processing task - should not release",
			taskID: "task5",
			userID: "user5",
			task: &model.PictureTask{
				TaskID: "task5",
				UserID: "user5",
				Status: int32(pb.WorkflowTaskStatus_WORKFLOW_TASK_STATUS_PROCESSING),
			},
			taskErr:       nil,
			expectRelease: false,
			degradeMode:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建 mock
			mockTaskDao := &MockTaskDao{}
			mockQuotaChecker := &MockQueueSlotReleaser{}

			// 设置期望
			mockTaskDao.On("GetTask", mock.Anything, tt.taskID).Return(tt.task, tt.taskErr)
			mockQuotaChecker.On("IsDegradeMode").Return(tt.degradeMode)

			if tt.expectRelease {
				mockQuotaChecker.On("ReleaseQueueSlot", mock.Anything, tt.userID, tt.taskID).Return(nil)
			}

			// 使用通用骨架 + 队列适配器进行组合测试
			base := reconciler.NewReconcilerBase(
				reconciler.SlotReconcilerConfig{
					Enabled:          true,
					Interval:         time.Minute,
					RedisScanCount:   1000,
					MaxTasksPerRound: 10000,
					WorkerPoolSize:   16,
				},
				rdb,
				nil,
				zap.NewNop(),
				&reconciler.QueueAdapter{TaskGetter: mockTaskDao.GetTask, Releaser: mockQuotaChecker},
				"queue_slot_reconcile",
				nil,
			)

			// 执行测试
			ctx := context.Background()
			base.ProcessOneForTest(ctx, tt.userID, tt.taskID)

			// 验证期望
			mockTaskDao.AssertExpectations(t)
			mockQuotaChecker.AssertExpectations(t)
		})
	}
}

// TestQueueSlotReconciler_ShouldSkipInDegradeMode 测试降级模式下的跳过逻辑
func TestQueueSlotReconciler_ShouldSkipInDegradeMode(t *testing.T) {
	tests := []struct {
		name        string
		degradeMode bool
		expectSkip  bool
	}{
		{
			name:        "degrade mode on - should skip",
			degradeMode: true,
			expectSkip:  true,
		},
		{
			name:        "degrade mode off - should not skip",
			degradeMode: false,
			expectSkip:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQuotaChecker := &MockQueueSlotReleaser{}
			mockQuotaChecker.On("IsDegradeMode").Return(tt.degradeMode)

			// 通过骨架 + 适配器组合进行降级测试
			base := reconciler.NewReconcilerBase(
				reconciler.SlotReconcilerConfig{Enabled: true, Interval: time.Minute, WorkerPoolSize: 1, MaxTasksPerRound: 1},
				nil,
				nil,
				zap.NewNop(),
				&reconciler.QueueAdapter{TaskGetter: nil, Releaser: mockQuotaChecker},
				"queue_slot_reconcile",
				nil,
			)
			// 直接调用骨架的 processOne 行为来观察跳过情况
			ctx := context.Background()
			if !tt.degradeMode {
				// 非降级模式下，任务为空将触发释放调用
				mockQuotaChecker.On("ReleaseQueueSlot", mock.Anything, "u", "t").Return(nil)
			}
			base.ProcessOneForTest(ctx, "u", "t")
			// 如果降级模式为真，应跳过释放；否则应触发一次释放
			if tt.degradeMode {
				mockQuotaChecker.AssertNotCalled(t, "ReleaseQueueSlot", mock.Anything, "u", "t")
			} else {
				mockQuotaChecker.AssertCalled(t, "ReleaseQueueSlot", mock.Anything, "u", "t")
				mockQuotaChecker.AssertNumberOfCalls(t, "ReleaseQueueSlot", 1)
			}
			mockQuotaChecker.AssertExpectations(t)
		})
	}
}

// TestQueueSlotReconciler_StartStop 测试启动停止逻辑
func TestQueueSlotReconciler_StartStop(t *testing.T) {
	// 启动 miniredis
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	// 创建 Redis 客户端
	rdb := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	mockTaskDao := &MockTaskDao{}
	mockQuotaChecker := &MockQueueSlotReleaser{}

	qreconciler := slot.NewQueueSlotReconciler(
		slot.SlotReconcilerConfig{
			Enabled:          true,
			Interval:         100 * time.Millisecond,
			RedisScanCount:   1000,
			MaxTasksPerRound: 10000,
			WorkerPoolSize:   16,
		},
		rdb,
		mockTaskDao,
		mockQuotaChecker,
		zap.NewNop(),
	)

	// 测试启动
	qreconciler.Start()

	// 等待一小段时间
	time.Sleep(50 * time.Millisecond)

	// 测试停止
	qreconciler.Stop()

	// 再次停止应该不会panic
	qreconciler.Stop()
}
