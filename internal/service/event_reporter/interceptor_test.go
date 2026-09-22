package event_reporter

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsv1 "va_visionai_server/pkg/event_sink/v1"
)

// ============ DefaultInterceptor 测试 ============

func TestDefaultInterceptor_FillsEmptyFields(t *testing.T) {
	interceptor := NewDefaultInterceptor("test-source", "test-node")

	event := &eventsv1.Event{
		EventType: "TEST_EVENT",
	}

	result := interceptor.Intercept(event)

	require.NotNil(t, result)
	assert.Equal(t, "test-source", result.GetSource())
	assert.NotZero(t, result.GetOccurredMs())
	assert.Equal(t, "test-node", result.GetServerContext().GetServerNode())
}

func TestDefaultInterceptor_PreservesExistingValues(t *testing.T) {
	interceptor := NewDefaultInterceptor("test-source", "test-node")

	existingTime := int64(1234567890)
	event := &eventsv1.Event{
		EventType:  "TEST_EVENT",
		Source:     "custom-source",
		OccurredMs: existingTime,
		ServerContext: &eventsv1.ServerContext{
			ServerNode: "custom-node",
		},
	}

	result := interceptor.Intercept(event)

	require.NotNil(t, result)
	assert.Equal(t, "custom-source", result.GetSource())
	assert.Equal(t, existingTime, result.GetOccurredMs())
	assert.Equal(t, "custom-node", result.GetServerContext().GetServerNode())
}

func TestDefaultInterceptor_NilEvent(t *testing.T) {
	interceptor := NewDefaultInterceptor("test-source", "test-node")

	result := interceptor.Intercept(nil)

	assert.Nil(t, result)
}

// ============ InterceptorFunc 测试 ============

func TestInterceptorFunc(t *testing.T) {
	called := false
	fn := InterceptorFunc(func(event *eventsv1.Event) *eventsv1.Event {
		called = true
		event.Source = "modified"
		return event
	})

	event := &eventsv1.Event{EventType: "TEST"}
	result := fn.Intercept(event)

	assert.True(t, called)
	assert.Equal(t, "modified", result.GetSource())
}

// ============ InterceptorChain 测试 ============

func TestInterceptorChain_ExecutesInOrder(t *testing.T) {
	var order []int

	i1 := InterceptorFunc(func(event *eventsv1.Event) *eventsv1.Event {
		order = append(order, 1)
		return event
	})
	i2 := InterceptorFunc(func(event *eventsv1.Event) *eventsv1.Event {
		order = append(order, 2)
		return event
	})
	i3 := InterceptorFunc(func(event *eventsv1.Event) *eventsv1.Event {
		order = append(order, 3)
		return event
	})

	chain := NewInterceptorChain(i1, i2, i3)
	event := &eventsv1.Event{EventType: "TEST"}

	chain.Intercept(event)

	assert.Equal(t, []int{1, 2, 3}, order)
}

func TestInterceptorChain_StopsOnNil(t *testing.T) {
	var order []int

	i1 := InterceptorFunc(func(event *eventsv1.Event) *eventsv1.Event {
		order = append(order, 1)
		return event
	})
	i2 := InterceptorFunc(func(event *eventsv1.Event) *eventsv1.Event {
		order = append(order, 2)
		return nil // 丢弃事件
	})
	i3 := InterceptorFunc(func(event *eventsv1.Event) *eventsv1.Event {
		order = append(order, 3)
		return event
	})

	chain := NewInterceptorChain(i1, i2, i3)
	event := &eventsv1.Event{EventType: "TEST"}

	result := chain.Intercept(event)

	assert.Nil(t, result)
	assert.Equal(t, []int{1, 2}, order) // i3 不应该被调用
}

func TestInterceptorChain_Add(t *testing.T) {
	chain := NewInterceptorChain()
	assert.Equal(t, 0, chain.Len())

	chain.Add(InterceptorFunc(func(e *eventsv1.Event) *eventsv1.Event { return e }))
	assert.Equal(t, 1, chain.Len())

	chain.Add(InterceptorFunc(func(e *eventsv1.Event) *eventsv1.Event { return e }))
	assert.Equal(t, 2, chain.Len())
}

func TestInterceptorChain_EmptyChain(t *testing.T) {
	chain := NewInterceptorChain()
	event := &eventsv1.Event{EventType: "TEST"}

	result := chain.Intercept(event)

	assert.Equal(t, event, result)
}

// ============ SamplingInterceptor 测试 ============

func TestSamplingInterceptor_FullSampling(t *testing.T) {
	interceptor := NewSamplingInterceptor(1.0) // 100% 保留

	for i := 0; i < 10; i++ {
		event := &eventsv1.Event{EventType: "TEST"}
		result := interceptor.Intercept(event)
		assert.NotNil(t, result, "event %d should be kept", i)
	}
}

func TestSamplingInterceptor_NoSampling(t *testing.T) {
	interceptor := NewSamplingInterceptor(0.0) // 0% 保留

	for i := 0; i < 10; i++ {
		event := &eventsv1.Event{EventType: "TEST"}
		result := interceptor.Intercept(event)
		assert.Nil(t, result, "event %d should be dropped", i)
	}
}

func TestSamplingInterceptor_PartialSampling(t *testing.T) {
	interceptor := NewSamplingInterceptor(0.5) // 50% 保留

	kept := 0
	total := 100
	for i := 0; i < total; i++ {
		event := &eventsv1.Event{EventType: "TEST"}
		if interceptor.Intercept(event) != nil {
			kept++
		}
	}

	// 50% 采样率，应该保留约一半
	assert.Positive(t, kept)
	assert.Less(t, kept, total)
}

func TestSamplingInterceptor_BoundaryRates(t *testing.T) {
	// 测试边界值处理
	interceptor1 := NewSamplingInterceptor(-0.5) // 应该被修正为 0
	interceptor2 := NewSamplingInterceptor(1.5)  // 应该被修正为 1

	event := &eventsv1.Event{EventType: "TEST"}

	assert.Nil(t, interceptor1.Intercept(event))
	assert.NotNil(t, interceptor2.Intercept(event))
}

// ============ FilterInterceptor 测试 ============

func TestFilterInterceptor_KeepsMatchingEvents(t *testing.T) {
	interceptor := NewFilterInterceptor(func(e *eventsv1.Event) bool {
		return e.GetEventType() == "KEEP"
	})

	keepEvent := &eventsv1.Event{EventType: "KEEP"}
	dropEvent := &eventsv1.Event{EventType: "DROP"}

	assert.NotNil(t, interceptor.Intercept(keepEvent))
	assert.Nil(t, interceptor.Intercept(dropEvent))
}

func TestFilterInterceptor_FilterByUserID(t *testing.T) {
	// 只保留有 UserID 的事件
	interceptor := NewFilterInterceptor(func(e *eventsv1.Event) bool {
		return e.GetUserId() != ""
	})

	withUser := &eventsv1.Event{EventType: "TEST", UserId: "user123"}
	withoutUser := &eventsv1.Event{EventType: "TEST"}

	assert.NotNil(t, interceptor.Intercept(withUser))
	assert.Nil(t, interceptor.Intercept(withoutUser))
}

// ============ 复合场景测试 ============

func TestInterceptorChain_ComplexScenario(t *testing.T) {
	// 模拟真实场景：默认填充 -> 过滤无效事件 -> 采样

	defaultInterceptor := NewDefaultInterceptor("my-service", "node-1")

	filterInterceptor := NewFilterInterceptor(func(e *eventsv1.Event) bool {
		return e.GetEventType() != "" // 过滤掉没有类型的事件
	})

	// 100% 采样，确保测试确定性
	samplingInterceptor := NewSamplingInterceptor(1.0)

	chain := NewInterceptorChain(defaultInterceptor, filterInterceptor, samplingInterceptor)

	// 测试正常事件
	event := &eventsv1.Event{EventType: "USER_LOGIN"}
	result := chain.Intercept(event)

	require.NotNil(t, result)
	assert.Equal(t, "my-service", result.GetSource())
	assert.Equal(t, "node-1", result.GetServerContext().GetServerNode())
	assert.NotZero(t, result.GetOccurredMs())

	// 测试无效事件被过滤
	invalidEvent := &eventsv1.Event{} // 无 EventType
	result2 := chain.Intercept(invalidEvent)
	assert.Nil(t, result2)
}

func TestDefaultInterceptor_TimestampPrecision(t *testing.T) {
	interceptor := NewDefaultInterceptor("test-source", "test-node")

	before := time.Now().UnixMilli()
	event := &eventsv1.Event{EventType: "TEST"}
	interceptor.Intercept(event)
	after := time.Now().UnixMilli()

	assert.GreaterOrEqual(t, event.GetOccurredMs(), before)
	assert.LessOrEqual(t, event.GetOccurredMs(), after)
}

// ============ SessionID 和 Sequence 测试 ============

func TestDefaultInterceptor_SessionID(t *testing.T) {
	interceptor := NewDefaultInterceptor("test-source", "test-node")

	event := &eventsv1.Event{EventType: "TEST"}
	result := interceptor.Intercept(event)

	require.NotNil(t, result)
	// session_id 格式: sess_{hostname}_{timestamp}
	assert.True(t, strings.HasPrefix(result.GetSessionId(), "sess_"), "session_id should start with 'sess_'")
	assert.NotEmpty(t, result.GetSessionId())
}

func TestDefaultInterceptor_SessionID_Consistent(t *testing.T) {
	interceptor := NewDefaultInterceptor("test-source", "test-node")

	// 同一个 interceptor 应该产生相同的 session_id
	event1 := &eventsv1.Event{EventType: "TEST1"}
	event2 := &eventsv1.Event{EventType: "TEST2"}

	interceptor.Intercept(event1)
	interceptor.Intercept(event2)

	assert.Equal(t, event1.GetSessionId(), event2.GetSessionId())
}

func TestDefaultInterceptor_SessionID_PreservesExisting(t *testing.T) {
	interceptor := NewDefaultInterceptor("test-source", "test-node")

	event := &eventsv1.Event{
		EventType: "TEST",
		SessionId: "custom-session-id",
	}

	result := interceptor.Intercept(event)

	require.NotNil(t, result)
	assert.Equal(t, "custom-session-id", result.GetSessionId())
}

func TestDefaultInterceptor_Sequence(t *testing.T) {
	interceptor := NewDefaultInterceptor("test-source", "test-node")

	event1 := &eventsv1.Event{EventType: "TEST1"}
	event2 := &eventsv1.Event{EventType: "TEST2"}
	event3 := &eventsv1.Event{EventType: "TEST3"}

	interceptor.Intercept(event1)
	interceptor.Intercept(event2)
	interceptor.Intercept(event3)

	// sequence 应该递增
	assert.Equal(t, int64(1), event1.GetSequence())
	assert.Equal(t, int64(2), event2.GetSequence())
	assert.Equal(t, int64(3), event3.GetSequence())
}

func TestDefaultInterceptor_Sequence_PreservesExisting(t *testing.T) {
	interceptor := NewDefaultInterceptor("test-source", "test-node")

	event := &eventsv1.Event{
		EventType: "TEST",
		Sequence:  999,
	}

	result := interceptor.Intercept(event)

	require.NotNil(t, result)
	assert.Equal(t, int64(999), result.GetSequence())
}

func TestDefaultInterceptor_Sequence_Concurrent(t *testing.T) {
	interceptor := NewDefaultInterceptor("test-source", "test-node")

	const numGoroutines = 100
	var wg sync.WaitGroup
	sequences := make(chan int64, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event := &eventsv1.Event{EventType: "TEST"}
			interceptor.Intercept(event)
			sequences <- event.GetSequence()
		}()
	}

	wg.Wait()
	close(sequences)

	// 收集所有 sequence
	seqSet := make(map[int64]bool)
	for seq := range sequences {
		seqSet[seq] = true
	}

	// 验证所有 sequence 都是唯一的
	assert.Len(t, seqSet, numGoroutines, "all sequences should be unique")

	// 验证 sequence 范围正确 (1 到 numGoroutines)
	for i := int64(1); i <= numGoroutines; i++ {
		assert.True(t, seqSet[i], "sequence %d should exist", i)
	}
}
