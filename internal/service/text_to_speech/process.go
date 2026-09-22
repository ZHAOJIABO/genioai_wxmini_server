package text_to_speech

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

func Process(ctx context.Context, conn *websocket.Conn, input []string, output chan []byte, speaker string, cancel func(), ttsEncoder vai.TTSEncoder) {
	sessionID := uuid.New().String()
	namespace := "BidirectionalTTS"
	encoder := strings.ToLower(ttsEncoder.String())
	if err := startTTSSession(conn, sessionID, namespace, &TTSReqParams{
		Speaker: speaker,
		AudioParams: &AudioParams{
			Format:     encoder,
			SampleRate: 24000,
		},
	}); err != nil {
		zlog.LogWithContext(ctx).Error("TTS Start Session Error", zap.Error(err))
	}
	channel := common.NewChannel(1, 2, 1*time.Second)

	go func() {
		for {
			select {
			case v, ok := <-channel.Channel:
				if !ok {
					if err := finishSession(conn, sessionID); err != nil {
						zlog.Logger.Error("Finish TTS Session", zap.Error(err))
					}
					return
				}
				text := v.(string)
				if err := sendTTSMessage(conn, sessionID, text, speaker, encoder); err != nil {
					zlog.Logger.Error("Send TTS message", zap.Error(err))
					cancel()
				}
			case <-ctx.Done():
				if err := finishSession(conn, sessionID); err != nil {
					zlog.Logger.Error("Finish TTS Session", zap.Error(err))
				}
				return
			}
		}
	}()
	go func() {
		for _, s := range input {
			channel.SendData(s)
		}
		utils.SafeCloseChan(channel.Channel)
	}()

	loopReceiveAudio(ctx, conn, output, cancel)
}

func loopReceiveAudio(ctx context.Context, conn *websocket.Conn, output chan<- []byte, cancel func()) {
	defer func() {
		utils.SafeCloseChan(output)
		cancel()
	}()

loopReceiveAudio:
	for {
		msg, err := receiveMessage(conn)
		if err != nil {
			zlog.LogWithContext(ctx).Error("Receive message error", zap.Error(err))
			break
		}
		switch msg.Type {
		case MsgTypeFullServer:
			if msg.Event == int32(EventSessionFinished) {
				zlog.LogWithContext(ctx).Info("TTS Task Receive Session Finished")
				break loopReceiveAudio
			}
		case MsgTypeAudioOnlyServer:
			select {
			case output <- msg.Payload:
			case <-ctx.Done():
				zlog.LogWithContext(ctx).Info("TTS Task Context Done")
				return
			case <-time.After(3 * time.Second):
				zlog.LogWithContext(ctx).Error("TTS LoopReceiveAudio Error", zap.Error(errors.New("Send MsgPayload Timeout")))
				return
			}

		case MsgTypeError:
			zlog.LogWithContext(ctx).Error(fmt.Sprintf("Receive Error message (code=%d): %s", msg.ErrorCode, msg.Payload))
		default:
			zlog.LogWithContext(ctx).Error(fmt.Sprintf("Received unexpected message type: %s", msg.Type))
		}
	}

	if err := finishConnection(conn); err != nil {
		zlog.LogWithContext(ctx).Error("TTS Task Finish connection Error", zap.Error(err))
	}
	zlog.LogWithContext(ctx).Info("TTS Task Finish connection")
}

func receiveMessage(conn *websocket.Conn) (*Message, error) {
	mt, frame, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return nil, fmt.Errorf("unexpected Websocket message type: %d", mt)
	}

	msg, _, err := Unmarshal(frame, ContainsSequence)
	if err != nil {
		if len(frame) > 500 {
			frame = frame[:500]
		}
		return nil, fmt.Errorf("unmarshal response message: %w", err)
	}
	return msg, nil
}

func finishConnection(conn *websocket.Conn) error {
	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("create FinishConnection request message: %w", err)
	}
	msg.Event = int32(EventFinishConnection)
	msg.Payload = []byte("{}")

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal FinishConnection request message: %w", err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("send FinishConnection request: %w", err)
	}

	// Read ConnectionStarted message.
	mt, frame, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read ConnectionFinished response: %w", err)
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return fmt.Errorf("unexpected Websocket message type: %d", mt)
	}

	msg, _, err = Unmarshal(frame, protocol.ContainsSequence)
	if err != nil {
		return fmt.Errorf("unmarshal ConnectionFinished response message: %w", err)
	}
	if msg.Type != MsgTypeFullServer {
		return fmt.Errorf("unexpected ConnectionFinished message type: %s", msg.Type)
	}
	if Event(msg.Event) != EventConnectionFinished {
		return fmt.Errorf("unexpected response event (%s) for FinishConnection request", Event(msg.Event))
	}

	return nil
}

func sendTTSMessage(conn *websocket.Conn, sessionID, text, speaker, ttsEncoder string) error {
	req := TTSRequest{
		Event:     int32(EventTaskRequest),
		Namespace: "BidirectionalTTS",
		ReqParams: &TTSReqParams{
			Text:    text,
			Speaker: speaker,
			AudioParams: &AudioParams{
				Format:     ttsEncoder,
				SampleRate: 24000,
			},
		},
	}
	payload, err := json.Marshal(&req)
	if err != nil {
		return fmt.Errorf("marshal TaskRequest request payload: %w", err)
	}

	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("create TaskRequest request message: %w", err)
	}
	msg.Event = req.Event
	msg.SessionID = sessionID
	msg.Payload = payload

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal TaskRequest request message: %w", err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("send TaskRequest request: %w", err)
	}
	zlog.Logger.Info("Submit TTS Task")
	return nil
}

func startTTSSession(conn *websocket.Conn, sessionID, namespace string, params *TTSReqParams) error {
	req := TTSRequest{
		Event:     int32(EventStartSession),
		Namespace: namespace,
		ReqParams: params,
	}
	payload, err := json.Marshal(&req)
	if err != nil {
		return fmt.Errorf("marshal StartSession request payload: %w", err)
	}

	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("create StartSession request message: %w", err)
	}
	msg.Event = req.Event
	msg.SessionID = sessionID
	msg.Payload = payload

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal StartSession request message: %w", err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("send StartSession request: %w", err)
	}

	// Read SessionStarted message.
	mt, frame, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read SessionStarted response: %w", err)
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return fmt.Errorf("unexpected Websocket message type: %d", mt)
	}

	// Validate SessionStarted message.
	msg, _, err = Unmarshal(frame, protocol.ContainsSequence)
	if err != nil {
		return fmt.Errorf("unmarshal SessionStarted response message: %w", err)
	}
	if msg.Type != MsgTypeFullServer {
		return fmt.Errorf("unexpected SessionStarted message type: %s", msg.Type)
	}
	if Event(msg.Event) != EventSessionStarted {
		return fmt.Errorf("unexpected response event (%s) for StartSession request", Event(msg.Event))
	}
	zlog.Logger.Info("TTS session started", zap.String("SessionID", msg.SessionID))

	return nil
}

func finishSession(conn *websocket.Conn, sessionID string) error {
	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("create FinishSession request message: %w", err)
	}
	msg.Event = int32(EventFinishSession)
	msg.SessionID = sessionID
	msg.Payload = []byte("{}")

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal FinishSession request message: %w", err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("send FinishSession request: %w", err)
	}

	return nil
}
func handleConnection(conn *websocket.Conn, event Event, expectedEvent Event, action string) error {
	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("create %s request message: %w", action, err)
	}
	msg.Event = int32(event)
	msg.Payload = []byte("{}")

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal %s request message: %w", action, err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("send %s request: %w", action, err)
	}

	mt, frame, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read %s response: %w", action, err)
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return fmt.Errorf("unexpected Websocket message type: %d", mt)
	}

	msg, _, err = Unmarshal(frame, protocol.ContainsSequence)
	if err != nil {
		return fmt.Errorf("unmarshal %s response message: %w", action, err)
	}
	if msg.Type != MsgTypeFullServer {
		return fmt.Errorf("unexpected %s message type: %s", action, msg.Type)
	}
	if Event(msg.Event) != expectedEvent {
		return fmt.Errorf("unexpected response event (%s) for %s request", Event(msg.Event), action)
	}

	return nil
}
