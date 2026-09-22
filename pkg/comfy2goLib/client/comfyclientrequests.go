package client

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/dundunHa/comfy2go/graphapi"
	"github.com/pkg/errors"
)

// internalOutputs 和 internalPromptHistoryItem 是用于解析来自 /history API 的
// 复杂嵌套 JSON 响应的内部辅助结构体。
type internalOutputs struct {
	Images *[]DataOutput `json:"images"`
}
type internalPromptHistoryItem struct {
	// The prompt is stored as an array layed out like this:
	// [
	// 	[0] index 		int,
	// 	[1] promptID 	string,
	// 	[2] prompt 		map[string]graphapi.PromptNode, // we'll ignore this
	// 	[3] extra_data 	graphapi.PromptExtraData,       // the graph is in here
	//  [4] outputs     []string 						// array of nodeIDs that have outputs
	// ]
	Prompt  []interface{}              `json:"prompt"`
	Outputs map[string]internalOutputs `json:"outputs"`
}

/*
@routes.get("/embeddings")
@routes.get("/extensions")
@routes.get("/view")
@routes.get("/view_metadata/{folder_name}")
@routes.get("/system_stats")
@routes.get("/prompt")
@routes.get("/object_info")
@routes.get("/object_info/{node_class}")
@routes.get("/history")
@routes.get("/history/{prompt_id}")
@routes.get("/queue")

@routes.post("/prompt")
@routes.post("/queue")
@routes.post("/interrupt")
@routes.post("/history")
@routes.post("/upload/image")
@routes.post("/upload/mask")
*/

func (c *ComfyClient) GetSystemStats() (*SystemStats, error) {
	err := c.CheckConnection()
	if err != nil {
		return nil, err
	}

	req, err := c.createRequest("GET",
		fmt.Sprintf("%s://%s/system_stats", c.httpProtocol, c.serverBaseAddress),
		nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	retv := &SystemStats{}
	err = json.Unmarshal(body, &retv)
	if err != nil {
		return nil, err
	}

	return retv, nil
}

// GetPromptHistoryByIndex retrieves all prompt history items, sorted by their execution index.
func (c *ComfyClient) GetPromptHistoryByIndex() ([]PromptHistoryItem, error) {
	history, err := c.GetAllPromptHistory()
	if err != nil {
		return nil, err
	}

	retv := make([]PromptHistoryItem, len(history))
	index := 0
	// ComfyUI does not recalculate the indicies of prompt history items,
	// so the indecies may not always be ordered 0..n
	// We'll create a slice out of the map items, and then sort them
	for _, h := range history {
		retv[index] = h
		index++
	}

	sort.Slice(retv, func(i, j int) bool {
		return retv[i].Index < retv[j].Index
	})

	return retv, nil
}

// GetPromptHistory fetches the history for a specific prompt ID.
func (c *ComfyClient) GetPromptHistory(promptID string) (*PromptHistoryItem, error) {
	req, err := c.createRequest("GET",
		fmt.Sprintf("%s://%s/history/%s", c.httpProtocol, c.serverBaseAddress, promptID),
		nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get prompt history, status: %d, body: %s", resp.StatusCode, string(body))
	}

	history := make(map[string]internalPromptHistoryItem)
	if err = json.Unmarshal(body, &history); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal prompt history")
	}

	ph, ok := history[promptID]
	if !ok {
		return nil, fmt.Errorf("prompt history for promptID '%s' not found in response", promptID)
	}

	return c.reconstructHistoryItem(promptID, ph)
}

// GetAllPromptHistory fetches the entire prompt history.
func (c *ComfyClient) GetAllPromptHistory() (map[string]PromptHistoryItem, error) {
	req, err := c.createRequest("GET",
		fmt.Sprintf("%s://%s/history", c.httpProtocol, c.serverBaseAddress),
		nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// we need to re-arrange the data into something more coherent
	// We're going to have to make an adapter that reconstructs an actual prompt
	// from the mangled data

	// read in the body, and deserialize to our temp internalPromptHistoryItem type
	body, _ := io.ReadAll(resp.Body)
	history := make(map[string]internalPromptHistoryItem)
	err = json.Unmarshal(body, &history)
	if err != nil {
		return nil, err
	}

	// try to reconstruct the data into PromptHistoryItem
	ret := make(map[string]PromptHistoryItem)
	for k, ph := range history {
		item, err := c.reconstructHistoryItem(k, ph)
		if err != nil {
			slog.Error("failed to reconstruct history item", "promptID", k, "error", err)
			continue
		}
		ret[k] = *item
	}
	return ret, nil
}

// reconstructHistoryItem is a helper function to convert the raw history item into a structured PromptHistoryItem
func (c *ComfyClient) reconstructHistoryItem(promptID string, ph internalPromptHistoryItem) (*PromptHistoryItem, error) {
	// index, ok := ph.Prompt[0].(float64)
	// if !ok {
	// 	return nil, errors.New("invalid index format in prompt history")
	// }

	// // extract the graph from ph.Prompt[3]["extra_pnginfo"]["workflow"]
	// extraData, ok := ph.Prompt[3].(map[string]interface{})
	// if !ok {
	// 	return nil, errors.New("invalid extra_data format in prompt history")
	// }

	// extraPngInfo, ok := extraData["extra_pnginfo"].(map[string]interface{})
	// if !ok {
	// 	return nil, errors.New("invalid extra_pnginfo format in prompt history")
	// }

	// workflow := extraPngInfo["workflow"]
	// gdata, err := json.Marshal(workflow)
	// if err != nil {
	// 	return nil, errors.Wrap(err, "failed to marshal workflow from history")
	// }

	// graph := &graphapi.Graph{}
	// err = json.Unmarshal(gdata, &graph)
	// if err != nil {
	// 	return nil, errors.Wrap(err, "failed to unmarshal workflow graph from history")
	// }

	// reconstruct
	item := &PromptHistoryItem{
		PromptID: promptID,
		// Index:    int(index),
		// Graph:    graph,
		Outputs: make(map[int][]DataOutput),
	}

	// rebuild the images output map
	for k, o := range ph.Outputs {
		oid, err := strconv.Atoi(k)
		if err != nil {
			slog.Warn("failed to convert output node ID to int", "nodeID", k, "error", err)
			continue
		}
		if o.Images != nil {
			item.Outputs[oid] = *o.Images
		}
	}
	return item, nil
}

// GetViewMetadata retrieves the '__metadata__' field in a safetensors file.
// checkpoints
// vae
// loras
// clip
// unet
// controlnet
// style_models
// clip_vision
// gligen
// configs
// hypernetworks
// upscale_models
// onnx
// fonts
func (c *ComfyClient) GetViewMetadata(folder string, file string) (string, error) {
	req, err := c.createRequest("GET",
		fmt.Sprintf("%s://%s/view_metadata/%s?filename=%s", c.httpProtocol, c.serverBaseAddress, folder, file),
		nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

// GetImage
func (c *ComfyClient) GetImage(image_data DataOutput) (*[]byte, error) {
	params := url.Values{}
	params.Add("filename", image_data.Filename)
	params.Add("subfolder", image_data.Subfolder)
	params.Add("type", image_data.Type)

	req, err := c.createRequest("GET",
		fmt.Sprintf("%s://%s/view?%s", c.httpProtocol, c.serverBaseAddress, params.Encode()),
		nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return &body, nil
}

// GetVideo
func (c *ComfyClient) GetVideo(video_data DataOutput) (*[]byte, error) {
	params := url.Values{}
	params.Add("filename", video_data.Filename)
	params.Add("subfolder", video_data.Subfolder)
	params.Add("type", video_data.Type)
	// params.Add("format", "video/h264-mp4")
	// params.Add("frame_rate", "16")

	req, err := c.createRequest("GET",
		fmt.Sprintf("%s://%s/viewvideo?%s", c.httpProtocol, c.serverBaseAddress, params.Encode()),
		nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return &body, nil
}

// GetEmbeddings retrieves the list of Embeddings models installed on the ComfyUI server.
func (c *ComfyClient) GetEmbeddings() ([]string, error) {
	req, err := c.createRequest("GET",
		fmt.Sprintf("%s://%s/embeddings", c.httpProtocol, c.serverBaseAddress),
		nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	retv := make([]string, 0)
	err = json.Unmarshal(body, &retv)
	if err != nil {
		return nil, err
	}

	return retv, nil
}

func (c *ComfyClient) GetQueueExecutionInfo() (*QueueExecInfo, error) {
	req, err := c.createRequest("GET",
		fmt.Sprintf("%s://%s/prompt", c.httpProtocol, c.serverBaseAddress),
		nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	queue_exec := &QueueExecInfo{}
	err = json.Unmarshal(body, &queue_exec)
	if err != nil {
		return nil, err
	}

	return queue_exec, nil
}

// GetExtensions retrieves the list of extensions installed on the ComfyUI server.
func (c *ComfyClient) GetExtensions() ([]string, error) {
	req, err := c.createRequest("GET",
		fmt.Sprintf("%s://%s/extensions", c.httpProtocol, c.serverBaseAddress),
		nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	retv := make([]string, 0)
	err = json.Unmarshal(body, &retv)
	if err != nil {
		return nil, err
	}

	return retv, nil
}

func (c *ComfyClient) GetObjectInfos() (*graphapi.NodeObjects, error) {
	// 1. 最快路径：检查内存缓存
	if c.objectInfo != nil {
		return c.objectInfo, nil
	}

	// 2. 次快路径：检查 JSON 字符串缓存
	if c.objectInfoJSONCache != "" {
		result := &graphapi.NodeObjects{}
		err := json.Unmarshal([]byte(c.objectInfoJSONCache), &result.Objects)
		if err == nil {
			result.PopulateInputProperties()
			c.objectInfo = result // 成功解析后，存入内存缓存
			return result, nil
		}
		// 如果 JSON 缓存无效，记录错误并继续执行网络调用
		slog.Error("failed to parse object_info from cache, falling back to API", "error", err)
	}

	// 3. 最慢路径：网络调用
	req, err := c.createRequest("GET",
		fmt.Sprintf("%s://%s/object_info", c.httpProtocol, c.serverBaseAddress),
		nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	result := &graphapi.NodeObjects{}
	err = json.Unmarshal(body, &result.Objects)
	if err != nil {
		return nil, err
	}

	result.PopulateInputProperties()
	c.objectInfo = result // 从网络获取成功后，同样存入内存缓存
	return result, nil
}

func (c *ComfyClient) QueuePrompt(graph *graphapi.Graph) (*QueueItem, error) {
	err := c.CheckConnection()
	if err != nil {
		return nil, err
	}

	prompt, err := graph.GraphToPrompt(c.clientid)
	if err != nil {
		return nil, err
	}
	// prevent a race where the ws may provide messages about a queued item before
	// we add the item to our internal map
	c.webSocket.LockRead()
	defer c.webSocket.UnlockRead()

	data, _ := json.Marshal(prompt)
	req, err := c.createRequest("POST",
		fmt.Sprintf("%s://%s/prompt", c.httpProtocol, c.serverBaseAddress),
		strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// create the queue item
	item := &QueueItem{
		Workflow: graph,
		Messages: make(chan PromptMessage),
	}

	err = json.Unmarshal(body, &item)
	if err != nil {
		// mmm-k, is it one of these:
		// {"error": {"type": "prompt_no_outputs",
		//				"message": "Prompt has no outputs",
		//				"details": "",
		//				"extra_info": {}
		//			  },
		// "node_errors": []
		// }
		perror := &PromptErrorMessage{}
		perr := json.Unmarshal(body, &perror)
		if perr != nil {
			// return the original error
			slog.Error("error unmarshalling prompt error", "body", string(body))
			return nil, err
		} else {
			return nil, errors.New(perror.Error.Message)
		}
	}
	c.queueditems[item.PromptID] = item
	return item, nil
}

func (c *ComfyClient) Interrupt() error {
	req, err := c.createRequest("POST",
		fmt.Sprintf("%s://%s/interrupt", c.httpProtocol, c.serverBaseAddress),
		strings.NewReader("{}"))
	if err != nil {
		return err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	io.ReadAll(resp.Body)
	return nil
}

func (c *ComfyClient) EraseHistory() error {
	data := "{\"clear\": \"clear\"}"
	req, err := c.createRequest("POST",
		fmt.Sprintf("%s://%s/history", c.httpProtocol, c.serverBaseAddress),
		strings.NewReader(data))
	if err != nil {
		return err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	io.ReadAll(resp.Body)
	return nil
}

func (c *ComfyClient) EraseHistoryItem(promptID string) error {
	item := fmt.Sprintf("{\"delete\": [\"%s\"]}", promptID)
	req, err := c.createRequest("POST",
		fmt.Sprintf("%s://%s/history", c.httpProtocol, c.serverBaseAddress),
		strings.NewReader(item))
	if err != nil {
		return err
	}

	resp, err := c.httpclient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	io.ReadAll(resp.Body)
	return nil
}

// createRequest creates an http.Request with auth header if configured
func (c *ComfyClient) createRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	if c.auth != nil {
		req.Header.Add("Authorization", "Basic "+c.getAuthHeader())
	}

	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}
