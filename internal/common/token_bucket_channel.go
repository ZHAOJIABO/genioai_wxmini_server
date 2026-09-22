package common

import (
	"time"
)

type TokenBucket struct {
	rate      int
	capacity  int
	tokens    int
	lastTime  time.Time
	tokenChan chan struct{}
}

func NewTokenBucket(rate, capacity int) *TokenBucket {
	tb := &TokenBucket{
		rate:      rate,
		capacity:  capacity,
		tokens:    capacity,
		lastTime:  time.Now(),
		tokenChan: make(chan struct{}, capacity),
	}

	go tb.generateTokens()
	return tb
}

func (tb *TokenBucket) generateTokens() {
	for {
		<-time.After(time.Second)
		tb.addTokens()
	}
}

func (tb *TokenBucket) addTokens() {
	tb.tokens += tb.rate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	for i := 0; i < tb.tokens; i++ {
		select {
		case tb.tokenChan <- struct{}{}:
		default:
			break
		}
	}
}

func (tb *TokenBucket) GetToken() {
	<-tb.tokenChan
}

func (tb *TokenBucket) GetTokenNonblocking() bool {
	select {
	case <-tb.tokenChan:
		return true
	default:
		return false
	}
}

type Channel struct {
	TokenBucket *TokenBucket
	delayTime   time.Duration
	Channel     chan interface{}
}

func NewChannel(rate, capacity int, delayTime time.Duration) *Channel {
	tb := NewTokenBucket(rate, capacity)
	return &Channel{
		TokenBucket: tb,
		delayTime:   delayTime,
		Channel:     make(chan interface{}, capacity),
	}
}

func (ch *Channel) SendData(data interface{}) {
	ch.TokenBucket.GetToken()
	ch.Channel <- data
}

func (ch *Channel) StartSending(dataSizes []interface{}) {
	for _, dataSize := range dataSizes {
		ch.SendData(dataSize)
	}
}
