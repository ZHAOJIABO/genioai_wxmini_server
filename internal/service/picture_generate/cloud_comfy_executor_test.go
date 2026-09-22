package picture_generate

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeReplacePlaceholders(t *testing.T) {
	executor := &CloudComfyExecutor{}

	tests := []struct {
		name            string
		workflowJson    string
		processedParams map[string]string
		expectedResult  string
		expectError     bool
	}{
		{
			name:         "正常参数替换",
			workflowJson: `{"text": "{prompt}", "image": "{image_url}"}`,
			processedParams: map[string]string{
				"prompt":    "测试提示",
				"image_url": "http://example.com/image.jpg",
			},
			expectedResult: `{"text": "测试提示", "image": "http://example.com/image.jpg"}`,
			expectError:    false,
		},
		{
			name:         "包含特殊字符的参数",
			workflowJson: `{"text": "{prompt}"}`,
			processedParams: map[string]string{
				"prompt": "测试\n提示\"with\\quotes",
			},
			expectedResult: `{"text": "测试\n提示\"with\\quotes"}`,
			expectError:    false,
		},
		{
			name:         "空参数",
			workflowJson: `{"text": "{prompt}"}`,
			processedParams: map[string]string{
				"prompt": "",
			},
			expectedResult: `{"text": ""}`,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.safeReplacePlaceholders(tt.workflowJson, tt.processedParams)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)

				// 验证结果是有效的JSON
				var tempMap map[string]interface{}
				err := json.Unmarshal([]byte(result), &tempMap)
				require.NoError(t, err, "结果应该是一个有效的JSON")
			}
		})
	}
}

func TestBuildParametersJson(t *testing.T) {
	executor := &CloudComfyExecutor{}

	// 使用用户提供的workflowJson作为测试数据
	workflowJson := `{
  "1": {
    "inputs": {
      "conditioning": [
        "14",
        0
      ]
    },
    "class_type": "ConditioningZeroOut",
    "_meta": {
      "title": "条件零化"
    }
  },
  "13": {
    "inputs": {
      "image": "{image_url}"
    },
    "class_type": "LoadImage",
    "_meta": {
      "title": "Loadimage1"
    }
  },
  "14": {
    "inputs": {
      "text": "{prompt}",
      "clip": [
        "10",
        0
      ]
    },
    "class_type": "CLIPTextEncode",
    "_meta": {
      "title": "CLIP Text Encode (Positive Prompt)"
    }
  }
}`

	processedParams := map[string]string{
		"image_url": "https://visionai-ugc-global.domobcdn.com/com.domob.piclib/upload/202509/img/8c44b1fed652df6b74da8d44680c80a1.jpg",
		"prompt":    "为我生成人物的角色设定（Character Design）\n\n比例设定（不同身高对比、头身比等）\n\n三视图（正面、侧面、背面）\n\n表情设定（Expression Sheet） → 就是你发的那种图\n\n动作设定（Pose Sheet） → 各种常见姿势\n\n服装设定（Costume Design）",
	}

	clientID := "test-client-123"

	// 测试构建参数JSON
	result, err := executor.buildParametersJson(workflowJson, clientID, processedParams)

	require.NoError(t, err, "构建参数JSON应该成功")
	assert.NotEmpty(t, result, "结果不应该为空")

	// 验证结果是有效的JSON
	var params map[string]interface{}
	err = json.Unmarshal([]byte(result), &params)
	require.NoError(t, err, "结果应该是一个有效的JSON")

	// 验证关键字段
	assert.Equal(t, clientID, params["client_id"])
	assert.Contains(t, params, "prompt")

	// 验证prompt字段是有效的JSON
	promptRaw := params["prompt"]
	if rawMsg, ok := promptRaw.(json.RawMessage); ok {
		var promptMap map[string]interface{}
		err := json.Unmarshal(rawMsg, &promptMap)
		require.NoError(t, err, "prompt字段应该包含有效的JSON")

		// 验证参数是否被正确替换
		assert.Contains(t, promptMap, "14")
		node14 := promptMap["14"].(map[string]interface{})
		inputs := node14["inputs"].(map[string]interface{})
		text := inputs["text"].(string)
		assert.Contains(t, text, "Character Design", "参数应该被正确替换")
		assert.NotContains(t, text, "\\n", "换行符应该被正确转义")
	}
}
