package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var catalogModelID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_./:-]{0,255}$`)

const openRouterEmbeddingModels = "https://openrouter.ai/api/v1/embeddings/models"

type embeddingModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength *int   `json:"contextLength"`
	PromptPrice   string `json:"promptPricePerToken"`
}

type terminalReader struct {
	reader *bufio.Reader
	file   *os.File
}

func (r *terminalReader) Read(p []byte) (int, error) { return r.reader.Read(p) }
func inputFile(in io.Reader) *os.File {
	if r, ok := in.(*terminalReader); ok {
		return r.file
	}
	if f, ok := in.(*os.File); ok {
		return f
	}
	return nil
}
func inputLine(in io.Reader) (string, error) {
	var reader *bufio.Reader
	if r, ok := in.(*terminalReader); ok {
		reader = r.reader
	} else if r, ok := in.(*bufio.Reader); ok {
		reader = r
	} else {
		reader = bufio.NewReader(in)
	}
	value := []byte{}
	for {
		part, err := reader.ReadSlice('\n')
		if len(value)+len(part) > 8193 {
			return "", errors.New("input line exceeds 8192 bytes")
		}
		value = append(value, part...)
		if err == bufio.ErrBufferFull {
			continue
		}
		return string(value), err
	}
}

func interactiveInput(in io.Reader) bool {
	f := inputFile(in)
	if f == nil {
		return false
	}
	s, err := f.Stat()
	return err == nil && s.Mode()&os.ModeCharDevice != 0
}

func (c Client) embeddingModels(keyEnv string) ([]embeddingModel, error) {
	address := openRouterEmbeddingModels
	if c.modelsURL != "" {
		address = c.modelsURL
	}
	request, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		return nil, errors.New("invalid model catalog endpoint")
	}
	// The public catalog currently works without auth; include a configured key
	// when available, as documented, but never log it or follow redirects with it.
	key, _, err := c.localKey(keyEnv)
	if err != nil {
		return nil, err
	}
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	request.Header.Set("Accept", "application/json")
	base := c.HTTP
	if base == nil {
		base = &http.Client{Timeout: 20 * time.Second}
	}
	client := *base
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("cannot load OpenRouter embedding models; check network access")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("OpenRouter model catalog answered HTTP %d; check lm.config and provider access", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return nil, errors.New("model catalog is unreadable or too large")
	}
	var catalog struct {
		Data []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Context *int   `json:"context_length"`
			Pricing struct {
				Prompt string `json:"prompt"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &catalog) != nil || catalog.Data == nil {
		return nil, errors.New("invalid embedding model catalog")
	}
	models := []embeddingModel{}
	seen := map[string]bool{}
	for _, m := range catalog.Data {
		if !catalogModelID.MatchString(m.ID) {
			continue
		}
		if seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		models = append(models, embeddingModel{m.ID, terminalText(m.Name), m.Context, m.Pricing.Prompt})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	if len(models) == 0 {
		return nil, errors.New("OpenRouter advertised no usable embedding models")
	}
	return models, nil
}
func renderModels(out io.Writer, models []embeddingModel) {
	fmt.Fprintln(out, "OpenRouter embedding models (live catalog; dimensions are observed by doctor --probe):")
	for i, m := range models {
		price := "price unknown"
		if p, err := strconv.ParseFloat(m.PromptPrice, 64); err == nil && p >= 0 && !math.IsInf(p, 0) && !math.IsNaN(p) {
			price = fmt.Sprintf("$%.6g / 1M input tokens", p*1000000)
		}
		fmt.Fprintf(out, "  %2d. %s  (%s)\n", i+1, terminalText(m.ID), price)
	}
}
func (c Client) chooseEmbeddingModel(keyEnv string, in io.Reader, prompt io.Writer) (string, error) {
	models, err := c.embeddingModels(keyEnv)
	if err != nil {
		return "", err
	}
	renderModels(prompt, models)
	fmt.Fprint(prompt, "Choose a model number or exact ID: ")
	line, err := inputLine(in)
	if err != nil && err != io.EOF {
		return "", errors.New("cannot read model selection")
	}
	choice := strings.TrimSpace(line)
	if index, err := strconv.Atoi(choice); err == nil && index >= 1 && index <= len(models) {
		return models[index-1].ID, nil
	}
	for _, m := range models {
		if choice == m.ID {
			return m.ID, nil
		}
	}
	return "", errors.New("choose a model from the live catalog; run lm models --provider openrouter --json for agent use")
}
