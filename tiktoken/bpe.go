package tiktoken

// Rewrittern from https://github.com/dmitry-brazhenko/SharpToken
// with the help of the ChatGPT

import (
	"bufio"
	"encoding/base64"
	"errors"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var reg = regexp.MustCompile(`(?i:'s|'t|'re|'ve|'m|'ll|'d)|[^\r\n\p{L}\p{N}]?\p{L}+|\p{N}{1,3}| ?[^\s\p{L}\p{N}]+[\r\n]*|\s*[\r\n]+|\s+`)

var specialTokens = map[string]int{
	"<|endoftext|>":   100257,
	"<|fim_prefix|>":  100258,
	"<|fim_middle|>":  100259,
	"<|fim_suffix|>":  100260,
	"<|im_start|>":    100264,
	"<|im_end|>":      100265,
	"<|endofprompt|>": 100276,
}

func ParseEncoder(r io.Reader) (map[string]int, error) {
	tokens := map[string]int{}
	s := bufio.NewScanner(r)
	for s.Scan() {
		l := s.Text()
		i := strings.Index(l, " ")
		b, err := base64.StdEncoding.DecodeString(l[:i])
		if err != nil {
			return nil, err
		}
		r, err := strconv.Atoi(l[i+1:])
		if err != nil {
			return nil, err
		}
		w := string(b)
		tokens[w] = r
	}
	return tokens, nil
}

type BPE struct {
	encoder        map[string]int
	specialEncoder map[string]int
	decoder        map[int]string
	specialDecoder map[int]string
	pattern        *regexp.Regexp
	specialPattern *regexp.Regexp
}

func NewCL100K(r io.Reader) (*BPE, error) {
	tokens, err := ParseEncoder(r)
	if err != nil {
		return nil, err
	}
	return NewBPE(tokens, specialTokens, reg)
}

func NewBPE(tokens map[string]int, specialTokens map[string]int, pattern *regexp.Regexp) (*BPE, error) {
	bpe := &BPE{
		encoder:        tokens,
		specialEncoder: specialTokens,
		decoder:        make(map[int]string),
		specialDecoder: make(map[int]string),
		pattern:        pattern,
	}

	for k, v := range tokens {
		bpe.decoder[v] = k
	}

	for k, v := range specialTokens {
		bpe.specialDecoder[v] = k
	}

	escapedTokens := make([]string, 0, len(specialTokens))
	for k := range specialTokens {
		escapedTokens = append(escapedTokens, regexp.QuoteMeta(k))
	}
	joinedParts := strings.Join(escapedTokens, "|")

	var err error
	bpe.specialPattern, err = regexp.Compile(joinedParts)
	if err != nil {
		return nil, errors.New("Invalid regular expression pattern")
	}

	return bpe, nil
}

func (b *BPE) EncodeNative(text string, allowedSpecial map[string]struct{}) ([]int, int, error) {
	encodedTokens := []int{}
	startIndex := 0
	lastTokenLength := 0

	for {
		nextSpecialStartIndex, err := b.nextSpecialStart(text, allowedSpecial, startIndex)
		if err != nil {
			return nil, 0, err
		}

		endIndex := len(text)
		if nextSpecialStartIndex != -1 {
			endIndex = nextSpecialStartIndex
		}

		textSegment := text[startIndex:endIndex]
		matches := b.pattern.FindAllString(textSegment, -1)

		for _, match := range matches {
			encodedPiece := match
			if token, ok := b.encoder[encodedPiece]; ok {
				lastTokenLength = 1
				encodedTokens = append(encodedTokens, token)
				continue
			}

			tokens, err := bytePairEncode(encodedPiece, b.encoder)
			if err != nil {
				return nil, 0, err
			}

			lastTokenLength = len(tokens)
			encodedTokens = append(encodedTokens, tokens...)
		}

		if nextSpecialStartIndex != -1 {
			specialToken := text[nextSpecialStartIndex:]
			specialTokenValue, ok := b.specialEncoder[specialToken]
			if !ok {
				return nil, 0, errors.New("Special token not found in SpecialTokensEncoder")
			}

			encodedTokens = append(encodedTokens, specialTokenValue)
			startIndex = nextSpecialStartIndex + len(specialToken)
			lastTokenLength = 0
		} else {
			break
		}
	}

	return encodedTokens, lastTokenLength, nil
}

func (b *BPE) nextSpecialStart(text string, allowedSpecial map[string]struct{}, startIndex int) (int, error) {
	searchIndex := startIndex

	for {
		nextSpecialMatch := b.specialPattern.FindStringIndex(text[searchIndex:])
		if nextSpecialMatch == nil {
			return -1, nil
		}

		specialToken := text[searchIndex+nextSpecialMatch[0] : searchIndex+nextSpecialMatch[1]]
		if _, ok := allowedSpecial[specialToken]; ok {
			return searchIndex + nextSpecialMatch[0], nil
		}

		searchIndex = searchIndex + nextSpecialMatch[0] + 1
	}
}

func (b *BPE) DecodeNative(tokens []int) []byte {
	decodedBytes := make([]byte, 0, len(tokens)*2)

	for _, token := range tokens {
		tokenBytes, ok := b.tryDecodeToken(token)
		if !ok {
			continue
		}

		decodedBytes = append(decodedBytes, []byte(tokenBytes)...)
	}

	return decodedBytes
}

func (b *BPE) tryDecodeToken(token int) (string, bool) {
	if decodedToken, ok := b.decoder[token]; ok {
		return decodedToken, true
	}

	if specialDecodedToken, ok := b.specialDecoder[token]; ok {
		return specialDecodedToken, true
	}

	return "", false
}

func (b *BPE) Count(s string) int {
	tks, _, _ := b.EncodeNative(s, nil)
	return len(tks)
}

// https://github.com/openai/openai-cookbook/blob/main/examples/How_to_count_tokens_with_tiktoken.ipynb
func (b *BPE) CountMessage(msgs []map[string]string) int {
	n := 0
	for _, m := range msgs {
		n += 4 // every message follows <|start|>{role/name}\n{content}<|end|>\n
		for k, v := range m {
			n += b.Count(v)
			// if there's a name, the role is omitted
			if k == "name" {
				n--
			}
		}
	}
	n += 3 // every reply is primed with <|start|>assistant<|message|>
	return n
}

func bytePairEncode(input string, bytePairRanks map[string]int) ([]int, error) {
	if len(input) == 1 {
		token := bytePairRanks[input]
		return []int{token}, nil
	}

	return bytePairMerge(input, bytePairRanks, func(r _Range) int {
		key := input[r.Start:r.End]
		return bytePairRanks[key]
	}), nil
}

type _Range struct {
	Start, End int
}

type _Part struct {
	Start, Rank int
}

func bytePairMerge(piece string, ranks map[string]int, f func(_Range) int) []int {
	partitions := make([]_Part, len(piece)+1)

	for i := range partitions {
		partitions[i].Start = i
		partitions[i].Rank = math.MaxInt
	}

	getRank := func(parts []_Part, start, skip int) int {
		if start+skip+2 >= len(parts) {
			return math.MaxInt
		}

		key := piece[parts[start].Start:parts[start+skip+2].Start]
		if rank, ok := ranks[key]; ok {
			return rank
		}
		return math.MaxInt
	}

	for i := 0; i < len(partitions)-2; i++ {
		partitions[i].Rank = getRank(partitions, i, 0)
	}

	for len(partitions) > 1 {
		minRank := math.MaxInt
		minRankIdx := 0

		for i := 0; i < len(partitions)-1; i++ {
			if partitions[i].Rank < minRank {
				minRank = partitions[i].Rank
				minRankIdx = i
			}
		}

		if minRank == math.MaxInt {
			break
		}

		partitions[minRankIdx].Rank = getRank(partitions, minRankIdx, 1)

		if minRankIdx > 0 {
			partitions[minRankIdx-1].Rank = getRank(partitions, minRankIdx-1, 1)
		}

		partitions = append(partitions[:minRankIdx+1], partitions[minRankIdx+2:]...)
	}

	output := make([]int, 0, len(partitions)-1)
	for i := 0; i < len(partitions)-1; i++ {
		output = append(output, f(_Range{partitions[i].Start, partitions[i+1].Start}))
	}

	return output
}
