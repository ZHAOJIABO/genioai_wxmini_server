package picture_generate

import (
	"reflect"
	"testing"
)

func TestCollectOrderedImageURLs(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]string
		want   []string
	}{
		{
			name:   "按编号排序而非字典序",
			params: map[string]string{"input_image_1": "a", "input_image_2": "b", "input_image_10": "c"},
			want:   []string{"a", "b", "c"},
		},
		{
			name:   "LoadImage 前缀",
			params: map[string]string{"LoadImage2": "b", "LoadImage1": "a"},
			want:   []string{"a", "b"},
		},
		{
			name:   "跳过空值与非图片入参",
			params: map[string]string{"input_image_1": "a", "input_image_2": "  ", "prompt": "p"},
			want:   []string{"a"},
		},
		{
			name:   "无编号按 1 处理",
			params: map[string]string{"LoadImage": "a", "input_image_2": "b"},
			want:   []string{"a", "b"},
		},
		{
			name:   "无图片入参",
			params: map[string]string{"prompt": "p"},
			want:   []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// map 遍历顺序随机，重复多次确认结果稳定
			for i := 0; i < 50; i++ {
				if got := collectOrderedImageURLs(c.params); !reflect.DeepEqual(got, c.want) {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}
