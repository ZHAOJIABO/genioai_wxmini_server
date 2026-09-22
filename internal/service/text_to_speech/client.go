package text_to_speech

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"va_visionai_server/conf"
	"va_visionai_server/internal/zlog"
)

var (
	protocol = NewBinaryProtocol()
)

func init() {
	// Initialize binary protocol settings.
	protocol.SetVersion(Version1)
	protocol.SetHeaderSize(HeaderSize4)
	protocol.SetSerialization(SerializationJSON)
	protocol.SetCompression(CompressionNone, nil)
	protocol.ContainsSequence = ContainsSequence
}

func NewClient(ctx context.Context, connID, logID, appID, accessKey string) (*websocket.Conn, error) {
	conn, err := dial(ctx, connID, logID, appID, accessKey)
	if err != nil {
		return nil, err
	}
	if err := startConnection(conn); err != nil {
		return nil, err
	}
	return conn, err
}

func buildHTTPHeader(connID, logID, appID, accessKey string) http.Header {
	h := http.Header{
		"X-Tt-Logid":        []string{logID},
		"X-Api-Resource-Id": []string{"volc.service_type.10029"},
		"X-Api-Access-Key":  []string{accessKey},
		"X-Api-App-Key":     []string{appID},
		"X-Api-Connect-Id":  []string{connID},
	}
	return h
}

func dial(ctx context.Context, connID, logID, appID, accessKey string) (*websocket.Conn, error) {
	config := conf.GlobalConfig.LlmConfig.VolcEngineTTS
	header := buildHTTPHeader(connID, logID, appID, accessKey)
	conn, r, connErr := websocket.DefaultDialer.DialContext(context.Background(), config.Endpoint, header)
	if connErr != nil {
		if r != nil {
			body, parseErr := io.ReadAll(r.Body)
			if parseErr != nil {
				parseErr = fmt.Errorf("parse response body failed: %w", parseErr)
				body = []byte(parseErr.Error())
			}
			connErr = fmt.Errorf("[code=%s] [body=%s] %w", r.Status, body, connErr)
		}
		return nil, connErr
	}
	defer r.Body.Close()

	zlog.LogWithContext(ctx).Info("Dial TTS Server ", zap.String("LogID", logID))
	return conn, nil
}

func startConnection(conn *websocket.Conn) error {
	msg, err := NewMessage(MsgTypeFullClient, MsgTypeFlagWithEvent)
	if err != nil {
		return fmt.Errorf("create StartSession request message: %w", err)
	}
	msg.Event = int32(EventStartConnection)
	msg.Payload = []byte("{}")

	frame, err := protocol.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal StartConnection request message: %w", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("send StartConnection request: %w", err)
	}

	// Read ConnectionStarted message.
	mt, frame, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read ConnectionStarted response: %w", err)
	}
	if mt != websocket.BinaryMessage && mt != websocket.TextMessage {
		return fmt.Errorf("unexpected Websocket message type: %d", mt)
	}
	msg, _, err = Unmarshal(frame, protocol.ContainsSequence)
	if err != nil {
		return fmt.Errorf("unmarshal ConnectionStarted response message: %w", err)
	}
	if msg.Type != MsgTypeFullServer {
		return fmt.Errorf("unexpected ConnectionStarted message type: %s", msg.Type)
	}
	if Event(msg.Event) != EventConnectionStarted {
		return fmt.Errorf("unexpected response event (%s) for StartConnection request", Event(msg.Event))
	}

	return nil
}
