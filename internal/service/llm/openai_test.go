package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectAndReplaceLatex(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantText     string
		wantFormulas []string
	}{
		{
			name:         "单行单个公式",
			input:        "这是一个公式 $E=mc^2$ 在文本中",
			wantText:     "这是一个公式 {{MATH0}} 在文本中",
			wantFormulas: []string{"E=mc^2"},
		},
		{
			name:         "多行公式",
			input:        "这是一个多行公式 $$\\sum_{i=1}^n i = \\frac{n(n+1)}{2}$$ 在文本中",
			wantText:     "这是一个多行公式 {{MATH0}} 在文本中",
			wantFormulas: []string{"\\sum_{i=1}^n i = \\frac{n(n+1)}{2}"},
		},
		{
			name:         "多个公式混合",
			input:        "第一个公式 $a^2$ 和第二个公式 $$\\int_0^\\infty e^{-x} dx$$ 在文本中",
			wantText:     "第一个公式 {{MATH0}} 和第二个公式 {{MATH1}} 在文本中",
			wantFormulas: []string{"a^2", "\\int_0^\\infty e^{-x} dx"},
		},
		{
			name:         "没有公式",
			input:        "这是一个普通文本，没有任何数学公式",
			wantText:     "这是一个普通文本，没有任何数学公式",
			wantFormulas: []string{},
		},
		{
			name:         "跨行公式",
			input:        "公式开始 $$\\begin{align}\ny &= mx + b\\\\\n&= 2x + 1\n\\end{align}$$ 公式结束",
			wantText:     "公式开始 {{MATH0}} 公式结束",
			wantFormulas: []string{"\\begin{align}\ny &= mx + b\\\\\n&= 2x + 1\n\\end{align}"},
		},
	}

	model := &OpenAIModel{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotFormulas := model.extractLatexContent(tt.input)
			assert.Equal(t, tt.wantText, gotText, "替换后的文本不匹配")
			assert.Equal(t, tt.wantFormulas, gotFormulas, "提取的公式不匹配")
		})
	}
}
