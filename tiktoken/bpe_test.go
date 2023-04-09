package tiktoken

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncode(t *testing.T) {
	f, err := os.Open("./cl100k_base.tiktoken")
	assert.Nil(t, err)
	b, err := NewCL100K(f)
	assert.Nil(t, err)
	f.Close()

	f, err = os.Open("./test_data.txt")
	assert.Nil(t, err)
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := s.Text()
		if strings.HasPrefix(l, "EncodingName") && strings.Contains(l, "cl100k_base") {
			s.Scan()
			l = s.Text()
			sample := l[len("Sample: "):]
			s.Scan()
			l = s.Text()
			encoded := l[len("Encoded: "):]
			var data []int
			err := json.Unmarshal([]byte(encoded), &data)
			assert.Nil(t, err)

			x, _, err := b.EncodeNative(sample, nil)
			assert.Nil(t, err)
			assert.Equal(t, data, x)
		}
	}
}

func TestCount(t *testing.T) {
	f, err := os.Open("./cl100k_base.tiktoken")
	assert.Nil(t, err)
	b, err := NewCL100K(f)
	assert.Nil(t, err)
	f.Close()

	msgs := []map[string]string{
		{
			"role":    "system",
			"content": "You are a helpful, pattern-following assistant that translates corporate jargon into plain English.",
		},
		{
			"role":    "system",
			"name":    "example_user",
			"content": "New synergies will help drive top-line growth.",
		},
		{
			"role":    "system",
			"name":    "example_assistant",
			"content": "Things working well together will increase revenue.",
		},
		{
			"role":    "system",
			"name":    "example_user",
			"content": "Let's circle back when we have more bandwidth to touch base on opportunities for increased leverage.",
		},
		{
			"role":    "system",
			"name":    "example_assistant",
			"content": "Let's talk later when we're less busy about how to do better.",
		},
		{
			"role":    "user",
			"content": "This late pivot means we don't have time to boil the ocean for the client deliverable.",
		},
	}

	assert.Equal(t, 127, b.CountMessage(msgs))
}

func TestCountStream(t *testing.T) {
	f, err := os.Open("./cl100k_base.tiktoken")
	assert.Nil(t, err)
	b, err := NewCL100K(f)
	assert.Nil(t, err)
	f.Close()

	msgs := []map[string]string{
		{
			"content": "李白静夜思",
			"role":    "user",
		},
	}

	assert.Equal(t, 17, b.CountMessage(msgs))

	n := b.Count("床前明月光，疑是地上霜。\n举头望明月，低头思故乡。")
	assert.Equal(t, 34, n)

	n = 0
	for _, s := range []string{
		"床", "前", "明", "月", "光", "，\n",
		"疑", "是", "地", "上", "霜", "。\n",
		"举", "头", "望", "明", "月", "，\n",
		"低", "头", "思", "故", "乡", "。",
	} {
		n += b.Count(s)
	}
	assert.Equal(t, 34, n)
}
