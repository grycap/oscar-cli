package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrDatasetNodeMissing is returned when the RO-Crate metadata lacks the dataset node.
	ErrDatasetNodeMissing = errors.New("ro-crate metadata missing dataset node")
	// ErrNoAcceptanceTests is returned when no acceptance tests are defined in the RO-Crate metadata.
	ErrNoAcceptanceTests = errors.New("no acceptance tests defined in the RO-Crate metadata")
	// ErrEmptyROCrate indicates that the parsed RO-Crate does not include entities.
	ErrEmptyROCrate = errors.New("ro-crate graph is empty")
)

// ROCrate represents the subset of an RO-Crate file that we need to inspect.
type ROCrate struct {
	Context any                      `json:"@context"`
	Graph   []map[string]interface{} `json:"@graph"`

	index map[string]map[string]interface{}
}

// AcceptanceTest captures the information required to execute a validation test.
type AcceptanceTest struct {
	ID                 string
	Name               string
	Command            string
	ExpectedSubstring  string
	Inputs             []TestInput
	RawNode            map[string]interface{}
	AdditionalSubjects []string
}

// TestInput describes an input artifact referenced by an acceptance test.
type TestInput struct {
	ID             string
	Name           string
	URL            string
	EncodingFormat string
}

// AcceptanceResult stores the outcome of an executed acceptance test.
type AcceptanceResult struct {
	Test    AcceptanceTest
	Passed  bool
	Output  string
	Details string
	Err     error
}

// ParseROCrate decodes a RO-Crate payload and indexes its entities.
func ParseROCrate(raw []byte) (*ROCrate, error) {
	var crate ROCrate
	if err := json.Unmarshal(raw, &crate); err != nil {
		return nil, fmt.Errorf("decoding ro-crate: %w", err)
	}
	if len(crate.Graph) == 0 {
		return nil, ErrEmptyROCrate
	}
	crate.buildIndex()
	return &crate, nil
}

func (c *ROCrate) buildIndex() {
	c.index = make(map[string]map[string]interface{})
	for _, node := range c.Graph {
		if node == nil {
			continue
		}
		if id, _ := node["@id"].(string); id != "" {
			c.index[id] = node
		}
	}
}

func (c *ROCrate) datasetNode() (map[string]interface{}, error) {
	if c.index == nil {
		c.buildIndex()
	}
	if dataset, ok := c.index["./"]; ok {
		return dataset, nil
	}
	return nil, ErrDatasetNodeMissing
}

// AcceptanceTests extracts the acceptance tests described in the RO-Crate metadata.
func (c *ROCrate) AcceptanceTests() ([]AcceptanceTest, error) {
	dataset, err := c.datasetNode()
	if err != nil {
		return nil, err
	}

	subjectIDs := extractIDs(dataset["subjectOf"])
	tests := make([]AcceptanceTest, 0, len(subjectIDs))

	for _, subjectID := range subjectIDs {
		node, ok := c.index[subjectID]
		if !ok {
			continue
		}

		command := readString(node, "command")
		expected := readString(node, "expectedSubstring")
		name := readString(node, "name")
		additional := extractIDs(node["subjectOf"])

		inputRefs := extractIDs(node["supply"])
		inputs := make([]TestInput, 0, len(inputRefs))
		for _, inputID := range inputRefs {
			resNode, ok := c.index[inputID]
			if !ok {
				continue
			}
			inputs = append(inputs, TestInput{
				ID:             inputID,
				Name:           readString(resNode, "name"),
				URL:            firstNonEmpty(readString(resNode, "contentUrl"), readString(resNode, "url")),
				EncodingFormat: readString(resNode, "encodingFormat"),
			})
		}

		tests = append(tests, AcceptanceTest{
			ID:                 subjectID,
			Name:               name,
			Command:            command,
			ExpectedSubstring:  expected,
			Inputs:             inputs,
			RawNode:            node,
			AdditionalSubjects: additional,
		})
	}

	if len(tests) == 0 {
		return nil, ErrNoAcceptanceTests
	}
	return tests, nil
}

func (c *ROCrate) entity(id string) map[string]interface{} {
	if c.index == nil {
		c.buildIndex()
	}
	return c.index[id]
}

func extractIDs(value interface{}) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		if v != "" {
			return []string{v}
		}
	case map[string]interface{}:
		if id, _ := v["@id"].(string); id != "" {
			return []string{id}
		}
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			result = append(result, extractIDs(item)...)
		}
		return result
	}
	return nil
}

func readString(node map[string]interface{}, key string) string {
	if node == nil {
		return ""
	}
	value, exists := node[key]
	if !exists {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case map[string]interface{}:
		if id, _ := v["@id"].(string); id != "" {
			return id
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
