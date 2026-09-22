package model

import (
	"errors"

	vai "va_visionai_server/internal/va_interface"
)

type MessageHistory struct {
	Sender    string
	Content   string
	URL       string
	URLs      []string
	FileType  vai.MessageType
	FileHash  string
	VoiceHash string
	VideoBlob []byte
}

type (
	Output   chan []string
	CancelCh chan struct{}
)

func (o Output) Read() ([]string, error) {
	if output, ok := <-o; ok {
		return output, nil
	}
	return nil, errors.New("EOF")
}
