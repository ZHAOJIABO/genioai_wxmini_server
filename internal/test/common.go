package test

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	vai "va_visionai_server/internal/va_interface"
)

// Getclient 已废弃，VisionAiService 已被移除
// 请使用具体的服务客户端，如 GetMediaClient, GetChatClient 等
func Getclient() error {
	return errors.New("VisionAiService has been removed, please use specific service clients")
}

func GetMediaClient() (vai.MediaServiceClient, *grpc.ClientConn, error) {
	serverAddr := flag.String("s", "127.0.0.1:8181", "server name")
	flag.Parse()
	conn, err := grpc.Dial(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("fail to dial %s\n%v\n", *serverAddr, err)
		conn.Close()
		return nil, nil, err
	}
	client := vai.NewMediaServiceClient(conn)
	return client, conn, nil
}

func GetHeader(isTemp bool) *vai.RequestHeader {
	header := vai.RequestHeader{
		Device: GetDevice(),
		ReqId:  time.Now().Format("20060102150405"),
	}
	if isTemp {
		header.UserType = vai.UserType_USER_TYPE_TEMP
		header.UserId = "8629535352438"
	} else {
		header.UserType = vai.UserType_USER_TYPE_REGISTER
		header.UserId = "8613630303024"

	}

	return &header
}

func GetDevice() *vai.Device {
	device := vai.Device{}
	device.Os = 2
	device.Idfv = "1111111111"
	return &device
}

func checkRspCode(rsp *vai.ResponseHeader) error {
	if rsp.GetCode() != vai.StatusCode_SUCCESS {
		return errors.New("Code Is Not Success")
	}

	return nil
}
