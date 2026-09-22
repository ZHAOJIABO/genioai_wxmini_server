package prometheus

import vai "va_visionai_server/internal/va_interface"

type Counter struct {
	Label []string
}

type UserTokenCounter struct {
	UserID    string
	Token     int32
	TokenType string
	ModelID   vai.Model
}

func (c *Counter) Inc() {
	if len(c.Label) > 0 {
		chatCounter.WithLabelValues(c.Label...).Inc()
	}
}

func (c *UserTokenCounter) Inc() {
	if c.UserID != "" && c.Token > 0 {
		tokenCounter.WithLabelValues(c.UserID, c.ModelID.String(), c.TokenType).Add(float64(c.Token))
	}
}
