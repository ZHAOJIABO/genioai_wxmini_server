//go:build client
// +build client

package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"va_visionai_server/internal/common"
	vai "va_visionai_server/internal/va_interface"
)

var globalClient vai.VisionAiServiceClient
var promptClient vai.PromptServiceClient
var ttsClient vai.TTSServiceClient
var mediaClient vai.MediaServiceClient
var refreshToken, accessToken, userId string
var online bool

var deviceIden string

func main() {
	// deviceIden = "111111274342332132"
	userId = "8632545365011"
	//accessToken = "fake"
	serverAddr := flag.String("s", "visual-ai-server-pre.tomato-visual.com:443", "server name")
	// serverAddr := flag.String("s", "localhost:8181", "server name")
	flag.Parse()
	if strings.Contains(*serverAddr, "domob.cn") {
		online = true
	}

	// 创建 TLS 配置
	config := &tls.Config{
		InsecureSkipVerify: false, // 生产环境应设置为 false 并提供有效的证书
	}

	// 使用 TLS 证书创建连接
	creds := credentials.NewTLS(config)
	conn, err := grpc.Dial(*serverAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		fmt.Printf("fail to dial %s\n%v\n", *serverAddr, err)
		return
	}
	globalClient = vai.NewVisionAiServiceClient(conn)
	_, err = globalClient.Ping(context.Background(), &vai.Empty{})
	if err != nil {
		fmt.Printf("server pong got failed :\n%v\n", err)
		return
	} else {
		fmt.Println("Server pong successfully.")
	}
	promptClient = vai.NewPromptServiceClient(conn)
	ttsClient = vai.NewTTSServiceClient(conn)
	mediaClient = vai.NewMediaServiceClient(conn)
	printUsage()
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		command := scanner.Text()
		handleCommand(command)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  login                - Login with phone number and verification code")
	fmt.Println("  relogin <user|token.go> - Login with user info")
	fmt.Println("  chatlist             - Get the list of chats")
	fmt.Println("  messagelist <chatId> - Get the list of messages in a chat")
	fmt.Println("  signin               - Sign in to receive daily rewards")
	fmt.Println("  ad                   - Receive ad rewards")
	fmt.Println("  award                - Get award information")
	fmt.Println("  userinfo             - Get user information")
	fmt.Println("  chat <chatId>        - Start chatting in a specific chat")
	fmt.Println("  logout               - Logout from the system")
	fmt.Println("  chatarchive <chatId> - Delete chat history by ChatID")
	fmt.Println("  upload <filePath>    - Upload an image/file")
	fmt.Println("  clear                - Delete All Chat history")
}

func handleCommand(command string) {
	args := strings.Split(command, " ")
	switch args[0] {
	case "promptlist":
		PromptList()
	case "tmplogin":
		handleTempLogin()
	case "getconfig":
		GetConfig(args[1])
	case "login":
		handleLogin()
	case "relogin":
		handleLoginByUserinfo()
	case "chatlist":
		if userId != "" {
			count := 20
			offset := 1
			if len(args) == 3 {
				count, _ = strconv.Atoi(args[2])
			}
			if len(args) >= 2 {
				offset, _ = strconv.Atoi(args[1])
			}
			err := GetChatList(userId, accessToken, offset, count)
			if err != nil {
				fmt.Printf("GetChatList failed: %v\n", err)
			}
		} else {
			fmt.Println("Please login first.")
		}
	case "messagelist":
		if len(args) < 2 {
			fmt.Println("Usage: messagelist <chatId> <offset> <count>")
			return
		}
		if userId != "" && accessToken != "" {
			count := 20
			offset := 1
			if len(args) == 4 {
				count, _ = strconv.Atoi(args[3])
			}
			if len(args) >= 3 {
				offset, _ = strconv.Atoi(args[2])
			}
			err := GetMessageList(userId, args[1], accessToken, int32(offset), int32(count))
			if err != nil {
				fmt.Printf("GetMessageList failed: %v\n", err)
			}
		} else {
			fmt.Println("Please login first.")
		}
	case "upload":
		if len(args) < 2 {
			fmt.Println("Usage: upload <filePath>")
			return
		}
		UploadImage(userId, args[1], accessToken)
		//if userId != "" && accessToken != "" {
		//	err := UploadImage(userId, args[1], accessToken)
		//	if err != nil {
		//		fmt.Printf("UploadImage failed: %v\n", err)
		//	}
		//} else {
		//	fmt.Println("Please login first.")
		//}
	case "signin":
		if userId != "" && accessToken != "" {
			err := Signin(userId, accessToken)
			if err != nil {
				fmt.Printf("Signin failed: %v\n", err)
			}
		} else {
			fmt.Println("Please login first.")
		}
	case "ad":
		if userId != "" && accessToken != "" {
			err := AdReward(userId, accessToken)
			if err != nil {
				fmt.Printf("AdReward failed: %v\n", err)
			}
		} else {
			fmt.Println("Please login first.")
		}
	case "award":
		if userId != "" && accessToken != "" {
			err := AwardInfo(userId, accessToken)
			if err != nil {
				fmt.Printf("Award Info failed: %v\n", err)
			}
		} else {
			fmt.Println("Please login first.")
		}
	case "userinfo":
		if userId != "" && accessToken != "" {
			err := GetUserInfo(userId, accessToken)
			if err != nil {
				fmt.Printf("GetUserInfo failed: %v\n", err)
			}
		} else {
			fmt.Println("Please login first.")
		}
	case "chatarchive":
		if len(args) < 2 {
			fmt.Println("Usage: chatarchive <chatId>")
			return
		}
		if userId != "" && accessToken != "" {
			err := ChatArchive(userId, args[1], accessToken)
			if err != nil {
				fmt.Printf("ChatArchive failed: %v\n", err)
			}
		} else {
			fmt.Println("Please login first.")
		}
	case "clear":
		if userId != "" && accessToken != "" {
			err := ClearHistory(userId, accessToken)
			if err != nil {
				fmt.Printf("ChatArchive failed: %v\n", err)
			}
		} else {
			fmt.Println("Please login first.")
		}
	case "chat":
		if len(args) < 2 {
			fmt.Println("Usage: chat <chatId>")
			return
		}
		if userId != "" {
			err := SendMessageStream(userId, args[1], accessToken)
			if err != nil {
				fmt.Printf("SendMessageStream failed: %v\n", err)
			}
		} else {
			fmt.Println("Please login first.")
		}
	case "logout":
		accessToken, userId = "", ""
		fmt.Println("Logged out successfully.")
	case "refresh":
		RefreshToken()
	case "msgaudio":
		MsgAudioStream()
	default:
		fmt.Println("Unknown command:", args[0])
	}
}

func getLoginInfoByFileCache() string {
	cacheFile := "userinfo.txt"
	if online {
		cacheFile = "userinfo-online.txt"
	}
	by, _ := os.ReadFile(cacheFile)
	return string(by)
}

func writeLoginInfoToFileCache(userInfoCache string) {
	cacheFile := "userinfo.txt"
	if online {
		cacheFile = "userinfo-online.txt"
	}
	os.WriteFile(cacheFile, []byte(userInfoCache), 0644)
}
func handleLoginByUserinfo() (resp *vai.UserInfoResponse) {
	userinfo := getLoginInfoByFileCache()
	if userinfo == "" {
		scanner := bufio.NewScanner(os.Stdin)
		fmt.Print("Not found Userinfo in cache, Enter Userinfo : ")
		scanner.Scan()
		userinfo = scanner.Text()
	}
	if userinfo == "" {
		fmt.Println("Userinfo is empty.")
		return
	}
	userinfoSplit := strings.Split(strings.TrimSpace(userinfo), "|")
	accessToken, userId, refreshToken = userinfoSplit[1], userinfoSplit[0], userinfoSplit[2]
	req := &vai.UserInfoRequest{
		RequestHeader: buildRequestHeader(),
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	resp, err := globalClient.GetUserInfo(context.Background(), req)
	if err != nil {
		fmt.Printf("UserInfo not invalid: %v\n", err)
		return
	}
	if resp.ResponseHeader.Code != 0 {
		fmt.Printf("UserInfo not invalid: Code: %v Msg: %v\n", resp.ResponseHeader.Code, resp.ResponseHeader.Msg)
		return
	}
	fmt.Printf("Logged in successfully. User ID: %s, Access Token: %s\n", userId, accessToken)
	return
}
func handleLogin() {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("Enter phone number : ")
	scanner.Scan()
	phoneNumber := scanner.Text()
	_, err := SendVerifyCode(phoneNumber)
	if err != nil {
		fmt.Printf("Failed to send verify code: %v\n", err)
		return
	}

	fmt.Print("Enter verification code: ")
	scanner.Scan()
	verificationCode := scanner.Text()

	accessToken, userId, err = Login(phoneNumber, verificationCode)
	if err != nil {
		fmt.Printf("Login failed: %v\n", err)
		return
	}
	writeLoginInfoToFileCache(userId + "|" + accessToken + "|" + refreshToken)
	fmt.Printf("Logged in successfully. User Login Info: %s|%s\n", userId, accessToken)
}

func handleTempLogin() {
	var err error
	if userId == "" {
		accessToken, userId, err = TempLogin()
		if err != nil {
			fmt.Printf("Login failed: %v\n", err)
			return
		}
	}

	//writeLoginInfoToFileCache(userId + "|" + accessToken + "|" + refreshToken)
	fmt.Printf("Logged in successfully. User Login Info: %s|%s\n", userId, accessToken)
}

func buildRequestHeader() *vai.RequestHeader {
	return &vai.RequestHeader{
		ReqId:       time.Now().Format("20060102150405"),
		UserId:      userId,
		AccessToken: accessToken,
		Device:      &vai.Device{Os: 1},
		App:         &vai.App{AppVersion: "1.0.0", PackageName: "com.domob.piclib"},
	}
}

func SendVerifyCode(phoneNumber string) (string, error) {
	if phoneNumber == "+8619396357938" {
		return "", nil
	}
	req := &vai.VerifyRequest{
		RequestHeader: buildRequestHeader(),
		PhoneNumber:   phoneNumber,
	}
	fmt.Println("SendVerifyCode Req", common.FormatRequest(req.RequestHeader))
	resp, err := globalClient.SendVerifyCode(context.Background(), req)
	if err != nil {
		return "", err
	}
	fmt.Println("SendVerifyCode Resp", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return "", errors.New(resp.ResponseHeader.Msg)
	}

	return resp.String(), nil
}

func Login(phoneNumber, vc string) (string, string, error) {
	req := &vai.LoginRequest{
		RequestHeader: buildRequestHeader(),
		PhoneNumber:   phoneNumber,
		VerifyCode:    vc,
	}
	fmt.Println("Login Req", common.FormatRequest(req.RequestHeader))
	resp, err := globalClient.Login(context.Background(), req)
	if err != nil {
		return "", "", err
	}
	fmt.Println("Login Resp", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return "", "", errors.New(resp.ResponseHeader.Msg)
	}
	fmt.Printf("Login Resp Body: AccessToken: %s / ExpireTime: %s / RefreshToken: %s UserInfo %v\n",
		resp.AccessToken, resp.ExpireTime, resp.RefreshToken, resp.UserInfo)
	refreshToken = resp.RefreshToken
	return resp.AccessToken, resp.UserInfo.UserId, nil
}

func PromptList() error {
	req := &vai.ListPromptsRequest{
		RequestHeader: buildRequestHeader(),
	}

	req.RequestHeader.UserType = vai.UserType_USER_TYPE_TEMP
	req.RequestHeader.Device.AndroidId = deviceIden
	fmt.Println("Login Req", common.FormatRequest(req.RequestHeader))
	resp, err := promptClient.ListPrompt(context.Background(), req)
	if err != nil {
		return err
	}
	fmt.Println("Login Resp", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return errors.New(resp.ResponseHeader.Msg)
	}
	fmt.Printf("List Resp:%v", resp.GetKinds())
	return nil
}

func TempLogin() (string, string, error) {
	req := &vai.LoginRequest{
		RequestHeader: buildRequestHeader(),
	}

	req.RequestHeader.UserType = vai.UserType_USER_TYPE_TEMP
	req.RequestHeader.Device.AndroidId = deviceIden
	fmt.Println("Login Req", common.FormatRequest(req.RequestHeader))
	resp, err := globalClient.Login(context.Background(), req)
	if err != nil {
		return "", "", err
	}
	fmt.Println("Login Resp", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return "", "", errors.New(resp.ResponseHeader.Msg)
	}
	fmt.Printf("Login Resp Body: AccessToken: %s / ExpireTime: %s / RefreshToken: %s UserInfo %v\n",
		resp.AccessToken, resp.ExpireTime, resp.RefreshToken, resp.UserInfo)
	refreshToken = resp.RefreshToken
	return resp.AccessToken, resp.UserInfo.UserId, nil
}

func GetUserInfo(userId, accessToken string) error {
	req := &vai.UserInfoRequest{
		RequestHeader: buildRequestHeader(),
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	fmt.Println("GetUserInfo Req", common.FormatRequest(req.RequestHeader))
	resp, err := globalClient.GetUserInfo(context.Background(), req)
	if err != nil {
		return err
	}
	fmt.Println("GetUserInfo Resp", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return errors.New(resp.ResponseHeader.Msg)
	}
	fmt.Printf("GetUserInfo Body %v \n", resp.UserInfo)
	return nil
}

func Signin(userId, accessToken string) error {
	req := &vai.SignInRequest{
		RequestHeader: buildRequestHeader(),
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	fmt.Println("Signin Req", common.FormatRequest(req.RequestHeader))
	resp, err := globalClient.SignIn(context.Background(), req)
	if err != nil {
		return err
	}
	fmt.Println("Signin Resp", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return errors.New(resp.ResponseHeader.Msg)
	}
	fmt.Printf("Signin AwardAmount %v TotalAmount %v \n", resp.AwardAmount, resp.TotalAmount)
	return nil
}

func AdReward(userId, accessToken string) error {
	req := &vai.AdAwardRequest{
		RequestHeader: buildRequestHeader(),
		ActionType:    vai.AdActionType_VideoActionType_PLAY,
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	fmt.Println("AdReward Req", common.FormatRequest(req.RequestHeader))
	resp, err := globalClient.AdAward(context.Background(), req)
	if err != nil {
		return err
	}
	fmt.Println("AdReward Resp", common.FormatResponse(resp.ResponseHeader))
	fmt.Printf("AdReward AwardAmount %d TotalAmount %d \n", resp.AwardAmount, resp.TotalAmount)
	return nil
}

func AwardInfo(userId, accessToken string) error {
	req := &vai.AmountRequest{
		RequestHeader: buildRequestHeader(),
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	fmt.Println("GetAmountInfo Req", common.FormatRequest(req.RequestHeader))
	resp, err := globalClient.GetAmountInfo(context.Background(), req)
	if err != nil {
		return err
	}
	fmt.Println("GetAmountInfo Resp", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return errors.New(resp.ResponseHeader.Msg)
	}
	fmt.Printf("GetAmountInfo Body: %v \n", resp)
	return nil
}

func SendMessage(userId, chatId, accessToken string) error {
	req := &vai.ChatMessageSendRequest{
		RequestHeader: buildRequestHeader(),
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	req.Message = &vai.Message{
		ChatId:      chatId,
		Sender:      vai.MessageSender_USER,
		MessageType: vai.MessageType_MT_TEXT,
		CreateTime:  time.Now().Format(common.TimestampFormat),
		Content:     "hello",
	}

	fmt.Println("SendMessage req", common.FormatRequest(req.RequestHeader))
	resp, err := globalClient.SendChatMessage(context.Background(), req)
	if err != nil {
		return err
	}
	fmt.Println("SendMessage resp header", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return errors.New(resp.ResponseHeader.Msg)
	}
	fmt.Printf("SendMessage: %v \n", resp.Message)
	return nil
}

func GetChatList(userId, accessToken string, offset, count int) error {
	req := &vai.ChatListRequest{
		RequestHeader: buildRequestHeader(),
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	req.Count = int32(count)
	req.Offset = int32(offset)
	fmt.Println("GetChatList Req", common.FormatRequest(req.RequestHeader))
	resp, err := globalClient.GetChatList(context.Background(), req)
	if err != nil {
		return err
	}
	fmt.Println("GetChatList Resp", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return errors.New(resp.ResponseHeader.Msg)
	}
	fmt.Printf("GetChatList count: %d \n", len(resp.Chats))
	for idx, msg := range resp.Chats {
		fmt.Printf("\tChat: Idx:%d, ID:%s, Title:%s,CreateTime:%s\n", idx, msg.ChatId, msg.Title, msg.CreateTime)
	}
	return nil
}
func GetMessageList(userId, chatId, accessToken string, offset, count int32) error {
	req := &vai.ChatMessageListRequest{
		RequestHeader: buildRequestHeader(),
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	req.Count = count
	req.Offset = offset
	req.ChatId = chatId
	fmt.Println("GetMessageList Req", common.FormatRequest(req.RequestHeader), "offset", req.Offset, "count", req.Count)
	resp, err := globalClient.GetChatMessage(context.Background(), req)
	if err != nil {
		return err
	}
	fmt.Println("GetMessageList Resp", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return errors.New(resp.ResponseHeader.Msg)
	}
	fmt.Printf("GetMessageList count %d\n", len(resp.Messages))
	for idx, msg := range resp.Messages {

		fmt.Printf("\tMessage: Idx:%d, ID:%s, Sender:%s,Conent:%s [url:%s], Time:%s\n, VoiceInfo:%v|%v\n", idx, msg.MessageId, msg.Sender.String(), msg.Content, msg.Url, msg.CreateTime, msg.VoiceInfo.Duration, msg.GetUrl())
	}
	return nil
}
func ChatArchive(userId, chatId, accessToken string) error {
	req := &vai.ChatArchiveRequest{
		RequestHeader: buildRequestHeader(),
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	req.ChatId = chatId
	fmt.Println("ChatArchive Req", common.FormatRequest(req.RequestHeader))
	resp, err := globalClient.ChatArchive(context.Background(), req)
	if err != nil {
		return err
	}
	fmt.Println("ChatArchive Resp", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return errors.New(resp.ResponseHeader.Msg)
	}
	return nil
}

func RefreshToken() {
	req := &vai.RefreshTokenRequest{
		RequestHeader: buildRequestHeader(),
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	req.RefreshToken = refreshToken
	if _, err := globalClient.RefreshToken(context.TODO(), req); err != nil {
		fmt.Println("refreshToken err", err)
	}
}

func ClearHistory(userId, accessToken string) error {
	req := &vai.ClearChatHistoryRequest{
		RequestHeader: buildRequestHeader(),
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	fmt.Println("ClearHistory Req", common.FormatRequest(req.RequestHeader))
	resp, err := globalClient.ClearChatHistory(context.Background(), req)
	if err != nil {
		return err
	}
	fmt.Println("ClearHistory Resp", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return errors.New(resp.ResponseHeader.Msg)
	}
	return nil
}
func MsgAudioStream() error {
	txt := "Привет,Привет,Привет,Привет"
	ttsid := "ICL_zh_female_bingruoshaonv_tob"
	req := &vai.TTSRequest{

		RequestHeader: buildRequestHeader(),
		Msg:           txt,
		Speaker:       ttsid,
		//TtsEncoder:    vai.TTSEncoder_PCM,
	}
	stream, err := ttsClient.GetMsgAudio(context.TODO(), req)
	if err != nil {
		fmt.Println(err)
	}
	var audio []byte
	for {
		res, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("error receiving stream:", err)
			break
		}
		fmt.Println("res:", res)
		//if res.GetResponseHeader().Code != vai.StatusCode_SUCCESS {
		//	fmt.Println("error receiving stream,code is not success:", res.GetResponseHeader().GetMsg())
		//	break
		//}
		if err == nil {
			audio = append(audio, res.Audio...)

		}
	}
	//os.WriteFile(ttsid+"_en.mp3", audio, 0633)
	os.WriteFile("test.mp3", audio, 0633)
	return nil
}

func SendMessageStream(userId, chatId, accessToken string) error {
	req := &vai.ChatMessageSendRequest{
		RequestHeader: buildRequestHeader(),
		Message: &vai.Message{
			ChatId:      chatId,
			Sender:      vai.MessageSender_USER,
			MessageType: vai.MessageType_MT_TEXT,
			CreateTime:  time.Now().Format(common.TimestampFormat),
			ModelId:     vai.Model_MODEL_GPT4O,
			//PromptId:    "FUN_2",
		},
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	// req.RequestHeader.
	// req.RequestHeader.Device = &vai.Device{Os: 3}
	//req.Message.ModelId = vai.Model_MODEL_GPT4O
	fmt.Print("开启聊天， (输入 'exit' 退出):\n")
	scanner := bufio.NewScanner(os.Stdin)
	msgGot := ""
	for {
		fmt.Print("User: ")
		input := ""
		if scanner.Scan() {
			input = scanner.Text()
			if input == "exit" {
				break
			}
		}
		if strings.Contains(input, "|") && strings.Contains(input, "http") {
			url := strings.TrimSpace(strings.Split(input, "|")[1])
			req.Message.Content = strings.Split(input, "|")[0]
			req.Message.Url = url
			if strings.HasSuffix(url, ".pdf") {
				req.Message.MessageType = vai.MessageType_MT_FILE
			} else if strings.HasSuffix(url, ".jpeg") || strings.HasSuffix(url, ".jpg") {
				req.Message.MessageType = vai.MessageType_MT_IMAGE
				//req.Message.VoiceInfo = &vai.VoiceInfo{Md5: "e38aa7f6329a188a993077d844b6e8b25e2f9cf0cb589fdeb94a577b0399df9a"}
			}
		} else if strings.HasPrefix(input, "voice://") {
			req.Message.MessageType = vai.MessageType_MT_TEXT
			req.Message.Url = ""
			req.Message.VoiceInfo = &vai.VoiceInfo{Md5: "e38aa7f6329a188a993077d844b6e8b25e2f9cf0cb589fdeb94a577b0399df9a"}
		} else {
			req.Message.MessageType = vai.MessageType_MT_TEXT
			req.Message.Url = ""
			req.Message.Content = input
		}

		md := metadata.New(map[string]string{
			"os": "ios",
		})
		ctx := metadata.NewOutgoingContext(context.Background(), md)

		stream, err := globalClient.SendChatMessageStream(ctx, req)
		if err != nil {
			fmt.Println("could not call SendChatMessageStream: ", err)
			return err
		}
		fmt.Print("Assistant: ")
		//htmlMsg := ""
		for {
			res, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				fmt.Println("error receiving stream:", err)
				break
			}
			fmt.Println("res:", res)
			if res.GetResponseHeader().Code != vai.StatusCode_SUCCESS {
				fmt.Println("error receiving stream,code is not success:", res.GetResponseHeader().GetMsg())
				break
			}
			if err == nil {
				fmt.Println(res.MessageToken, res.IsEnd)
				fmt.Println("_____", res.GetResponseHeader().GetMsg())
				msgGot = msgGot + res.MessageToken
				//if res.IsEnd {
				//	htmlMsg = res.HtmlContent
				//}

			}
		}
		fmt.Println(msgGot)
		msgGot = ""
	}

	fmt.Println("结束对话")
	return nil
}

func UploadImage(userId, filePath, accessToken string) error {
	req := &vai.UploadRequest{
		RequestHeader: &vai.RequestHeader{},
	}
	req.RequestHeader.UserId = userId
	req.RequestHeader.AccessToken = accessToken
	req.FileName = filePath
	req.FileType = vai.FileType_FT_IMAGE

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}
	req.Data = data
	req.FileSize = int64(len(data))
	fmt.Println("UploadImage Req", common.FormatRequest(req.RequestHeader))
	resp, err := mediaClient.UploadImage(context.Background(), req)
	if err != nil {
		return err
	}
	fmt.Println("UploadImage Resp", common.FormatResponse(resp.ResponseHeader))
	if resp.ResponseHeader.Code != vai.StatusCode_SUCCESS {
		return errors.New(resp.ResponseHeader.Msg)
	}
	//fmt.Printf("File URL %s, low %s, size %dpx*%dpx\n", resp.FileUrl, resp.LowQualityUrl, resp.ImageWidth, resp.GetImageHeight())
	return nil
}

func GetConfig(key string) {
	req := &vai.FeedbackConfigRequest{RequestHeader: buildRequestHeader()}
	resp, err := globalClient.GetFeedBackConfig(context.TODO(), req)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(resp.PopupInterval, resp.PopupCount)
}
