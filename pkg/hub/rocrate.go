package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
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
	Steps              []AcceptanceStep
}

// AcceptanceStep represents an executable action within an acceptance test.
type AcceptanceStep struct {
	ID                string
	Name              string
	Command           string
	ExpectedSubstring string
	Inputs            []TestInput
	RawNode           map[string]interface{}
	ParsedCommand     *parsedCommand
	ExpectedMedia     []string
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
	Test        AcceptanceTest
	Passed      bool
	Output      string
	Details     string
	Err         error
	StepResults []AcceptanceStepResult
}

// AcceptanceStepResult stores the outcome of an executed acceptance step.
type AcceptanceStepResult struct {
	Step    AcceptanceStep
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
		inputs := c.testInputs(node)

		steps := make([]AcceptanceStep, 0, 1+len(extractIDs(node["hasPart"])))
		if strings.TrimSpace(command) != "" {
			steps = append(steps, AcceptanceStep{
				ID:                subjectID,
				Name:              name,
				Command:           command,
				ExpectedSubstring: expected,
				Inputs:            inputs,
				RawNode:           node,
			})
		}

		for _, partID := range extractIDs(node["hasPart"]) {
			partNode, ok := c.index[partID]
			if !ok {
				continue
			}
			step := AcceptanceStep{
				ID:                partID,
				Name:              readString(partNode, "name"),
				Command:           readString(partNode, "command"),
				ExpectedSubstring: readString(partNode, "expectedSubstring"),
				Inputs:            c.testInputs(partNode),
				RawNode:           partNode,
			}
			steps = append(steps, step)
		}

		steps = append(steps, c.parseStructuredSteps(subjectID, node)...)

		test := AcceptanceTest{
			ID:                 subjectID,
			Name:               name,
			Command:            command,
			ExpectedSubstring:  expected,
			Inputs:             inputs,
			RawNode:            node,
			AdditionalSubjects: additional,
			Steps:              steps,
		}

		if test.ExpectedSubstring == "" {
			for _, step := range test.Steps {
				if strings.TrimSpace(step.ExpectedSubstring) != "" {
					test.ExpectedSubstring = step.ExpectedSubstring
					break
				}
			}
		}

		tests = append(tests, test)
	}

	if len(tests) == 0 {
		return nil, ErrNoAcceptanceTests
	}
	return tests, nil
}

func (c *ROCrate) testInputs(node map[string]interface{}) []TestInput {
	inputRefs := extractIDs(node["supply"])
	if len(inputRefs) == 0 {
		return nil
	}

	inputs := make([]TestInput, 0, len(inputRefs))
	for _, inputID := range inputRefs {
		if input, ok := c.buildTestInput(inputID); ok {
			inputs = append(inputs, input)
		}
	}
	return inputs
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

func (c *ROCrate) parseStructuredSteps(testID string, node map[string]interface{}) []AcceptanceStep {
	rawSteps := node["step"]
	if rawSteps == nil {
		return nil
	}

	stepNodes := normalizeToSlice(rawSteps)
	if len(stepNodes) == 0 {
		return nil
	}

	type positionedStep struct {
		pos  int
		step AcceptanceStep
		idx  int
	}

	parsed := make([]positionedStep, 0, len(stepNodes))

	for idx, raw := range stepNodes {
		stepMap, ok := raw.(map[string]interface{})
		if !ok {
			if id, ok := raw.(string); ok {
				if resolved := c.resolveEntityMap(id); resolved != nil {
					stepMap = resolved
				} else {
					continue
				}
			} else {
				continue
			}
		} else if resolved := c.resolveEntityMap(stepMap); resolved != nil {
			stepMap = resolved
		}

		stepID := readString(stepMap, "@id")
		if strings.TrimSpace(stepID) == "" {
			stepID = fmt.Sprintf("%s#step%d", testID, idx+1)
		}

		stepName := firstNonEmpty(readString(stepMap, "name"), readString(stepMap, "text"), stepID)

		position := parsePosition(stepMap["position"])

		step := AcceptanceStep{
			ID:      stepID,
			Name:    stepName,
			RawNode: stepMap,
		}

		if paMap := c.resolveEntityMap(stepMap["potentialAction"]); paMap != nil {
			step.Command = c.commandTemplate(paMap["additionalProperty"])
			step.ExpectedSubstring = c.resolveExpectedSubstring(paMap)
			step.ExpectedMedia = c.resolveExpectedMediaTypes(paMap)
			step.Inputs = append(step.Inputs, c.stepInputs(paMap)...)

			if parsedCmd, ok := c.buildParsedCommand(paMap, step.Inputs); ok {
				step.ParsedCommand = parsedCmd
			}
		} else if duration := strings.TrimSpace(readString(stepMap, "timeRequired")); duration != "" {
			if parsedCmd, err := buildWaitCommand(duration); err == nil {
				step.ParsedCommand = parsedCmd
				step.Command = fmt.Sprintf("wait %s", parsedCmd.WaitDuration)
			}
		}

		parsed = append(parsed, positionedStep{
			pos:  position,
			step: step,
			idx:  idx,
		})
	}

	sort.SliceStable(parsed, func(i, j int) bool {
		if parsed[i].pos == parsed[j].pos {
			return parsed[i].idx < parsed[j].idx
		}
		if parsed[i].pos == 0 {
			return false
		}
		if parsed[j].pos == 0 {
			return true
		}
		return parsed[i].pos < parsed[j].pos
	})

	steps := make([]AcceptanceStep, 0, len(parsed))
	for _, item := range parsed {
		steps = append(steps, item.step)
	}
	return steps
}

func (c *ROCrate) resolveExpectedSubstring(action map[string]interface{}) string {
	expectedIDs := extractIDs(action["result"])
	for _, id := range expectedIDs {
		if node := c.entity(id); node != nil {
			if value := readString(node, "value"); value != "" {
				return value
			}
		}
	}
	return ""
}

func (c *ROCrate) resolveExpectedMediaTypes(action map[string]interface{}) []string {
	expectedIDs := extractIDs(action["result"])
	if len(expectedIDs) == 0 {
		return nil
	}

	var media []string
	for _, id := range expectedIDs {
		node := c.entity(id)
		if node == nil {
			continue
		}

		raw := node["encodingFormat"]
		switch v := raw.(type) {
		case string:
			if mt := strings.TrimSpace(v); mt != "" {
				media = append(media, mt)
			}
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					if mt := strings.TrimSpace(s); mt != "" {
						media = append(media, mt)
					}
				}
			}
		}
	}

	if len(media) == 0 {
		return nil
	}
	return media
}

func (c *ROCrate) stepInputs(action map[string]interface{}) []TestInput {
	objectIDs := extractIDs(action["object"])
	if len(objectIDs) == 0 {
		return nil
	}
	inputs := make([]TestInput, 0, len(objectIDs))
	for _, id := range objectIDs {
		if input, ok := c.buildTestInput(id); ok {
			inputs = append(inputs, input)
		} else if strings.TrimSpace(id) != "" {
			inputs = append(inputs, TestInput{ID: id})
		}
	}
	return inputs
}

func (c *ROCrate) buildParsedCommand(action map[string]interface{}, inputs []TestInput) (*parsedCommand, bool) {
	name := strings.ToLower(strings.TrimSpace(readString(action, "name")))
	template := c.commandTemplate(action["additionalProperty"])
	objectIDs := extractIDs(action["object"])

	switch name {
	case "run":
		directive := inputDirective{Mode: inputModeFile}
		if len(objectIDs) > 0 {
			directive.Value = objectIDs[0]
		} else if len(inputs) > 0 {
			directive.Value = inputs[0].ID
		}
		return &parsedCommand{
			Kind:         stepCommandRun,
			RunDirective: directive,
		}, true
	case "put-file":
		local := ""
		if len(objectIDs) > 0 {
			local = objectIDs[0]
		} else if len(inputs) > 0 {
			local = inputs[0].ID
		}
		return &parsedCommand{
			Kind:      stepCommandPutFile,
			LocalPath: local,
		}, true
	case "get-file":
		return &parsedCommand{
			Kind:            stepCommandGetFile,
			LatestRequested: strings.Contains(template, "--download-latest-into"),
		}, true
	}

	if strings.Contains(template, "service run") {
		return &parsedCommand{
			Kind:         stepCommandRun,
			RunDirective: inputDirective{Mode: inputModeFile, Value: firstObjectID(objectIDs, inputs)},
		}, true
	}
	if strings.Contains(template, "put-file") {
		return &parsedCommand{
			Kind:      stepCommandPutFile,
			LocalPath: firstObjectID(objectIDs, inputs),
		}, true
	}
	if strings.Contains(template, "get-file") {
		return &parsedCommand{
			Kind:            stepCommandGetFile,
			LatestRequested: strings.Contains(template, "--download-latest-into"),
		}, true
	}

	return nil, false
}

func firstObjectID(objectIDs []string, inputs []TestInput) string {
	if len(objectIDs) > 0 && strings.TrimSpace(objectIDs[0]) != "" {
		return objectIDs[0]
	}
	if len(inputs) > 0 {
		return inputs[0].ID
	}
	return ""
}

func readMap(node map[string]interface{}, key string) map[string]interface{} {
	value, ok := node[key]
	if !ok {
		return nil
	}
	switch t := value.(type) {
	case map[string]interface{}:
		return t
	}
	return nil
}

func (c *ROCrate) commandTemplate(raw interface{}) string {
	nodes := c.propertyNodes(raw)
	for _, node := range nodes {
		if propertyID := strings.TrimSpace(readString(node, "propertyID")); strings.EqualFold(propertyID, "commandTemplate") {
			if value := strings.TrimSpace(readString(node, "value")); value != "" {
				return value
			}
		}
	}
	return ""
}

func (c *ROCrate) resolveEntityMap(value interface{}) map[string]interface{} {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		if id := strings.TrimSpace(v); id != "" {
			return c.entity(id)
		}
	case map[string]interface{}:
		if id := strings.TrimSpace(readString(v, "@id")); id != "" {
			if node := c.entity(id); node != nil {
				return node
			}
		}
		return v
	case []interface{}:
		for _, item := range v {
			if resolved := c.resolveEntityMap(item); resolved != nil {
				return resolved
			}
		}
	}
	return nil
}

func (c *ROCrate) propertyNodes(raw interface{}) []map[string]interface{} {
	switch v := raw.(type) {
	case nil:
		return nil
	case string:
		if id := strings.TrimSpace(v); id != "" {
			if node := c.entity(id); node != nil {
				return []map[string]interface{}{node}
			}
		}
	case map[string]interface{}:
		if id := strings.TrimSpace(readString(v, "@id")); id != "" {
			if node := c.entity(id); node != nil {
				return []map[string]interface{}{node}
			}
		}
		return []map[string]interface{}{v}
	case []interface{}:
		var nodes []map[string]interface{}
		for _, item := range v {
			nodes = append(nodes, c.propertyNodes(item)...)
		}
		return nodes
	}
	return nil
}

func parsePosition(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case float64:
		if v == 0 {
			return 0
		}
		return int(math.Round(v))
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return 0
}

func buildWaitCommand(raw string) (*parsedCommand, error) {
	duration, err := parseISODuration(raw)
	if err != nil {
		return nil, err
	}
	return &parsedCommand{
		Kind:         stepCommandWait,
		WaitDuration: duration,
	}, nil
}

var isoDurationRegex = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

func parseISODuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	matches := isoDurationRegex.FindStringSubmatch(value)
	if matches == nil {
		return 0, fmt.Errorf("unsupported ISO 8601 duration: %s", value)
	}

	var (
		days, hours, minutes, seconds int
		err                           error
	)

	if matches[1] != "" {
		days, err = strconv.Atoi(matches[1])
		if err != nil {
			return 0, err
		}
	}
	if matches[2] != "" {
		hours, err = strconv.Atoi(matches[2])
		if err != nil {
			return 0, err
		}
	}
	if matches[3] != "" {
		minutes, err = strconv.Atoi(matches[3])
		if err != nil {
			return 0, err
		}
	}
	if matches[4] != "" {
		seconds, err = strconv.Atoi(matches[4])
		if err != nil {
			return 0, err
		}
	}

	total := (time.Duration(days) * 24 * time.Hour) +
		(time.Duration(hours) * time.Hour) +
		(time.Duration(minutes) * time.Minute) +
		(time.Duration(seconds) * time.Second)
	return total, nil
}

func normalizeToSlice(value interface{}) []interface{} {
	switch v := value.(type) {
	case []interface{}:
		return v
	case map[string]interface{}:
		return []interface{}{v}
	default:
		return nil
	}
}

func (c *ROCrate) buildTestInput(id string) (TestInput, bool) {
	if strings.TrimSpace(id) == "" {
		return TestInput{}, false
	}

	node := c.entity(id)
	if node == nil {
		return TestInput{ID: id}, true
	}

	if nodeHasType(node, "HowToSupply") {
		itemIDs := extractIDs(node["item"])
		for _, itemID := range itemIDs {
			if input, ok := c.buildTestInput(itemID); ok {
				return input, true
			}
		}
		return TestInput{}, false
	}

	return TestInput{
		ID:             id,
		Name:           readString(node, "name"),
		URL:            firstNonEmpty(readString(node, "contentUrl"), readString(node, "url")),
		EncodingFormat: readString(node, "encodingFormat"),
	}, true
}

func nodeHasType(node map[string]interface{}, target string) bool {
	rawType, ok := node["@type"]
	if !ok {
		return false
	}
	switch v := rawType.(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(v), target)
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				if strings.EqualFold(strings.TrimSpace(s), target) {
					return true
				}
			}
		}
	}
	return false
}
