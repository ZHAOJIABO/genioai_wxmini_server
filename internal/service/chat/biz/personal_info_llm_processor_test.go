package biz

import (
	"context"
	"strings"
	"testing"
)

// 简单分片与 suffix 保留测试
func TestCallbackProcessor_SimpleFlushAndSuffix(t *testing.T) {
	proc := newCallbackProcessor()

	// 第一个小 token，不足以 flush
	send1, stop1 := proc.process(context.TODO(), "Hi")
	if send1 != "" || stop1 {
		t.Fatalf("期望 send=\"\" stop=false，结果 send=%q stop=%v", send1, stop1)
	}
	if proc.suffix != "Hi" {
		t.Errorf("期望 suffix=Hi，结果 %q", proc.suffix)
	}

	// 第二个小 token，依然不足
	send2, stop2 := proc.process(context.TODO(), " there")
	if send2 != "" || stop2 {
		t.Fatalf("期望 send=\"\" stop=false，结果 send=%q stop=%v", send2, stop2)
	}
	if proc.suffix != "Hi there" {
		t.Errorf("期望 suffix=Hi there，结果 %q", proc.suffix)
	}

	// 第三个 token 导致 flush
	longToken := " this is a long token exceeding threshold"
	send3, stop3 := proc.process(context.TODO(), longToken)
	data := "Hi there" + longToken
	expectedFlush := data[:len(data)-17] // keep = 18-1 = 17
	if send3 != expectedFlush || stop3 {
		t.Errorf("flush 期望 %q stop=false，结果 send=%q stop=%v", expectedFlush, send3, stop3)
	}
	expectedSuffix := data[len(data)-17:]
	if proc.suffix != expectedSuffix {
		t.Errorf("期望 suffix=%q，结果 %q", expectedSuffix, proc.suffix)
	}
}

// 单独处理 ###answer 标记
func TestCallbackProcessor_AnswerMarker(t *testing.T) {
	proc := newCallbackProcessor()
	token := "prefix ###answersecret content"
	send, stop := proc.process(context.TODO(), token)
	if stop {
		t.Errorf("Answer 场景不应 stop，got %v", stop)
	}
	if !proc.processed["###answer"] {
		t.Errorf("应标记 processed[###answer]=true")
	}
	if strings.Contains(send, "###answer") {
		t.Errorf("send 不应包含 ###answer，got %q", send)
	}
	expectedSuffix := strings.ReplaceAll(token, "###answer", "")
	if proc.suffix != expectedSuffix {
		t.Errorf("期望 suffix=%q，结果 %q", expectedSuffix, proc.suffix)
	}
}

// 遇到 ###follow_question 时立即截断并 stop
func TestCallbackProcessor_FollowQuestion(t *testing.T) {
	proc := newCallbackProcessor()
	token := "hello###follow_questionworld"
	send, stop := proc.process(context.TODO(), token)
	if !stop || send != "hello" {
		t.Errorf("期望 send=hello stop=true，got send=%q stop=%v", send, stop)
	}
	// 再次调用应无输出且持续 stop
	send2, stop2 := proc.process(context.TODO(), "anything")
	if send2 != "" || !stop2 {
		t.Errorf("stop 后应无输出且持续 stop，got send=%q stop=%v", send2, stop2)
	}
}

// 同一 token 同时包含两种标记
func TestCallbackProcessor_BothMarkers(t *testing.T) {
	proc := newCallbackProcessor()
	token := "abc###answerdef###follow_questionghi"
	send, stop := proc.process(context.TODO(), token)
	if !proc.processed["###answer"] || !proc.processed["###follow_question"] {
		t.Error("应同时标记 answer 和 follow_question 已处理")
	}
	if !stop || send != "abcdef" {
		t.Errorf("期望 send=abcdef stop=true，got send=%q stop=%v", send, stop)
	}
}

// 标记跨边界拆分测试
func TestCallbackProcessor_MarkerAcrossBoundary(t *testing.T) {
	proc := newCallbackProcessor()
	send1, stop1 := proc.process(context.TODO(), "###ans")
	if send1 != "" || stop1 {
		t.Errorf("分片跨边界时不应立即 send，got send=%q stop=%v", send1, stop1)
	}
	if proc.suffix != "###ans" {
		t.Errorf("期望 suffix 保留###ans，got %q", proc.suffix)
	}

	_, stop2 := proc.process(context.TODO(), "wer rest")
	if stop2 {
		t.Errorf("跨边界组装后不应 stop，got %v", stop2)
	}
	if !proc.processed["###answer"] {
		t.Errorf("组装后应标记 processed[###answer]")
	}
	expectedSuffix := strings.ReplaceAll("###answer rest", "###answer", "")
	if proc.suffix != expectedSuffix {
		t.Errorf("期望 suffix=%q，got %q", expectedSuffix, proc.suffix)
	}
}

// 中文场景：当累计字符数小于 keep 时，应全部缓存，不产生 flush
func TestCallbackProcessor_ChineseNoFlush(t *testing.T) {
	proc := newCallbackProcessor()
	token := strings.Repeat("汉", 5) // 5 个汉字
	send, stop := proc.process(context.TODO(), token)
	if send != "" || stop {
		t.Fatalf("中文场景（不 flush）期望 send=\"\" stop=false，got send=%q stop=%v", send, stop)
	}
	if proc.suffix != token {
		t.Errorf("期望 suffix=%q，got %q", token, proc.suffix)
	}
}

// 中文场景：当累计字符数超过 keep（17）时，应按字符边界正确 flush
func TestCallbackProcessor_ChineseFlush(t *testing.T) {
	proc := newCallbackProcessor()
	token := strings.Repeat("汉", 20) // 20 个汉字
	send, stop := proc.process(context.TODO(), token)
	if stop {
		t.Fatalf("中文场景（flush）不应 stop，got %v", stop)
	}
	// keep = len("###follow_question")-1 = 18-1 = 17
	expectedSend := strings.Repeat("汉", 20-17) // 前 3 个汉字
	if send != expectedSend {
		t.Errorf("期望 send=%q，got %q", expectedSend, send)
	}
	expectedSuffix := strings.Repeat("汉", 17) // 后 17 个汉字
	if proc.suffix != expectedSuffix {
		t.Errorf("期望 suffix=%q，got %q", expectedSuffix, proc.suffix)
	}
}
