package test

import (
	"fmt"
	"image"
	"log"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/dundunHa/comfy2go/client"
	"github.com/schollz/progressbar/v3"
	"go.uber.org/zap"

	"va_visionai_server/internal/zlog"
)

func TestImg2Img(t *testing.T) {
	// 创建带有详细日志的回调
	callbacks := &client.ComfyClientCallbacks{
		ClientQueueCountChanged: func(c *client.ComfyClient, queuecount int) {
			log.Printf("[Queue] Client %s at %s Queue size changed to: %d",
				c.ClientID(), "120.133.56.109", queuecount)
		},
		// 添加其他可能的回调函数来跟踪客户端状态
	}

	// 初始化客户端
	log.Println("Creating new ComfyUI client")
	c := client.NewComfyClientWithTimeout("120.133.56.109", 8902, callbacks, 10, 3)

	// 加载工作流
	log.Println("Loading workflow file")
	graph, _, err := c.NewGraphFromJsonFile("/Users/lxp/Downloads/workflow.json")
	if err != nil {
		log.Println("Failed to load workflow", zap.Error(err))
		return
	}
	log.Println("Workflow loaded successfully",
		zap.Int("totalNodes", len(graph.Nodes)))

	// 客户端初始化
	if !c.IsInitialized() {
		log.Println("Initializing client", zap.String("clientID", c.ClientID()))
		err := c.Init()
		if err != nil {
			log.Println("Client initialization failed", zap.Error(err))
			return
		}
		log.Println("Client initialized successfully")
	}

	// 查找并处理所有 Load Image 节点
	nodes := graph.GetNodesWithTitle("Load Image")
	log.Println("Found Load Image nodes", zap.Int("count", len(nodes)))

	if len(nodes) == 0 {
		log.Println("No Load Image nodes found in workflow")
		return
	}

	// 处理每个 Load Image 节点
	for i, node := range nodes {
		log.Println("Processing Load Image node",
			zap.Int("index", i),
			zap.Int("nodeID", node.ID))

		prop := node.GetPropertyWithName("image")
		if prop == nil {
			log.Println("Missing image property in node",
				zap.Int("nodeID", node.ID))
			continue
		}

		uploadprop, _ := prop.ToImageUploadProperty()
		if uploadprop == nil {
			log.Println("Failed to get upload property", zap.Error(err))
			continue
		}

		// 加载和处理图片
		imgFile, err := os.Open("/Users/lxp/Downloads/sunset.png")
		if err != nil {
			log.Println("Failed to open image file", zap.Error(err))
			return
		}
		defer imgFile.Close()

		img, format, err := image.Decode(imgFile)
		if err != nil {
			log.Println("Failed to decode image",
				zap.Error(err),
				zap.String("format", format))
			return
		}

		// 上传图片
		filename := fmt.Sprintf("lxp_2_00000%d.png", i)
		log.Println("Uploading image",
			zap.String("filename", filename),
			zap.Int("nodeID", node.ID))

		_, err = c.UploadImage(img, filename, false, client.InputImageType, "", uploadprop)
		if err != nil {
			log.Println("Image upload failed", zap.Error(err))
			return
		}
		log.Println("Image uploaded successfully", zap.String("filename", filename))
	}

	// 提交工作流
	log.Println("Queueing workflow prompt")
	item, err := c.QueuePrompt(graph)
	if err != nil {
		log.Println("Failed to queue prompt", zap.Error(err))
		return
	}
	log.Println("Workflow queued successfully", zap.String("promptID", item.PromptID))

	// 处理消息
	var bar *progressbar.ProgressBar = nil
	var currentNodeTitle string

	timeout := time.After(5 * time.Minute)
	log.Println("Starting message processing loop")

	for continueLoop := true; continueLoop; {
		select {
		case msg, ok := <-item.Messages:
			if !ok {
				log.Println("Message channel closed unexpectedly")
				return
			}

			// 记录收到的每个消息
			log.Println("Received message",
				zap.String("type", msg.Type),
				zap.Any("rawContent", msg))

			switch msg.Type {
			case "started":
				qm := msg.ToPromptMessageStarted()
				log.Println("Workflow execution started",
					zap.String("promptID", qm.PromptID))

			case "executing":
				bar = nil
				qm := msg.ToPromptMessageExecuting()
				currentNodeTitle = qm.Title
				log.Println("Executing node",
					zap.Int("nodeID", qm.NodeID),
					zap.String("title", qm.Title))

			case "progress":
				qm := msg.ToPromptMessageProgress()
				log.Println("Progress update",
					zap.Int("value", qm.Value),
					zap.Int("max", qm.Max),
					zap.String("node", currentNodeTitle))
				if bar == nil {
					bar = progressbar.Default(int64(qm.Max), currentNodeTitle)
				}
				bar.Set(qm.Value)

			case "stopped":
				qm := msg.ToPromptMessageStopped()
				if qm.Exception != nil {
					log.Println("Workflow stopped with exception",
						zap.Any("exception", qm.Exception))
					return
				}
				log.Println("Workflow completed successfully")
				continueLoop = false

			case "data":
				qm := msg.ToPromptMessageData()
				log.Println("Received data message",
					zap.Any("dataKeys", reflect.ValueOf(qm.Data).MapKeys()))

				for k, v := range qm.Data {
					if k == "images" || k == "gifs" {
						for _, output := range v {
							log.Println("Processing output file",
								zap.String("filename", output.Filename),
								zap.String("type", output.Type),
								zap.String("subfolder", output.Subfolder))

							img_data, err := c.GetImage(output)
							if err != nil {
								log.Println("Failed to get image", zap.Error(err))
								continue
							}

							f, err := os.Create(output.Filename)
							if err != nil {
								log.Println("Failed to create output file", zap.Error(err))
								continue
							}

							_, err = f.Write(*img_data)
							if err != nil {
								zlog.Logger.Error("Failed to write image data", zap.Error(err))
								f.Close()
								continue
							}

							f.Close()
							zlog.Logger.Info("Successfully saved output file",
								zap.String("filename", output.Filename))
						}
					}
				}
			}

		case <-timeout:
			zlog.Logger.Error("Workflow execution timed out")
			return
		}
	}
}
