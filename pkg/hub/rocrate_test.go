package hub

import (
	"testing"
	"time"
)

func TestAcceptanceTestsIncludesStructuredSteps(t *testing.T) {
	raw := []byte(`{
		"@graph": [
			{
				"@id": "./",
				"subjectOf": [{ "@id": "#acceptance" }]
			},
			{
				"@id": "input.txt",
				"@type": "File",
				"name": "Sample Input",
				"url": "https://example.invalid/input.txt",
				"encodingFormat": "text/plain"
			},
			{
			"@id": "#expected-output",
			"@type": "PropertyValue",
			"value": "Characters: 44",
			"encodingFormat": "image/png"
			},
				{
					"@id": "#acceptance",
					"@type": "HowTo",
					"name": "Composite Test",
					"supply": [{ "@id": "#supply-input" }],
					"step": [
						{ "@id": "#step-run" },
						{ "@id": "#step-wait" },
						{ "@id": "#step-get" }
					]
				},
				{
					"@id": "#step-run",
					"@type": "HowToStep",
					"position": 1,
					"potentialAction": { "@id": "#action-run" }
				},
				{
					"@id": "#step-wait",
					"@type": "HowToStep",
					"position": 2,
					"timeRequired": "PT5S"
				},
				{
					"@id": "#step-get",
					"@type": "HowToStep",
					"position": 3,
					"potentialAction": { "@id": "#action-get" }
				},
			{
				"@id": "#supply-input",
				"@type": "HowToSupply",
				"item": { "@id": "input.txt" }
			},
			{
				"@id": "#action-run",
				"@type": "ConsumeAction",
				"name": "run",
				"object": { "@id": "input.txt" },
				"result": { "@id": "#expected-output" },
				"additionalProperty": [
					{ "@id": "#command-template-run" }
				]
			},
			{
				"@id": "#action-get",
				"@type": "TransferAction",
				"name": "get-file",
				"result": { "@id": "#expected-output" },
				"additionalProperty": [
					{ "@id": "#command-template-get" }
				]
			},
			{
				"@id": "#command-template-run",
				"@type": "PropertyValue",
				"propertyID": "commandTemplate",
				"value": "oscar-cli service run demo -i {input}"
			},
			{
				"@id": "#command-template-get",
				"@type": "PropertyValue",
				"propertyID": "commandTemplate",
				"value": "oscar-cli service get-file demo --download-latest-into {destination}"
			}
		]
	}`)

	crate, err := ParseROCrate(raw)
	if err != nil {
		t.Fatalf("ParseROCrate returned error: %v", err)
	}

	tests, err := crate.AcceptanceTests()
	if err != nil {
		t.Fatalf("AcceptanceTests returned error: %v", err)
	}

	if len(tests) != 1 {
		t.Fatalf("expected 1 acceptance test, got %d", len(tests))
	}

	test := tests[0]
	if len(test.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(test.Steps))
	}

	runStep := test.Steps[0]
	if runStep.ParsedCommand == nil || runStep.ParsedCommand.Kind != stepCommandRun {
		t.Fatalf("expected first step to be run command, got %+v", runStep.ParsedCommand)
	}
	if runStep.ParsedCommand.RunDirective.Mode != inputModeFile || runStep.ParsedCommand.RunDirective.Value != "input.txt" {
		t.Fatalf("unexpected run directive: %+v", runStep.ParsedCommand.RunDirective)
	}
	if runStep.ExpectedSubstring != "Characters: 44" {
		t.Fatalf("expected substring Characters: 44, got %q", runStep.ExpectedSubstring)
	}

	waitStep := test.Steps[1]
	if waitStep.ParsedCommand == nil || waitStep.ParsedCommand.Kind != stepCommandWait {
		t.Fatalf("expected second step to be wait command, got %+v", waitStep.ParsedCommand)
	}
	if waitStep.ParsedCommand.WaitDuration != 5*time.Second {
		t.Fatalf("expected wait duration 5s, got %s", waitStep.ParsedCommand.WaitDuration)
	}

	getStep := test.Steps[2]
	if getStep.ParsedCommand == nil || getStep.ParsedCommand.Kind != stepCommandGetFile {
		t.Fatalf("expected third step to be get-file command, got %+v", getStep.ParsedCommand)
	}
	if !getStep.ParsedCommand.LatestRequested {
		t.Fatalf("expected LatestRequested to be true for get-file step")
	}
	if getStep.ExpectedSubstring != "Characters: 44" {
		t.Fatalf("expected substring for get-file step, got %q", getStep.ExpectedSubstring)
	}
	if len(getStep.ExpectedMedia) != 1 || getStep.ExpectedMedia[0] != "image/png" {
		t.Fatalf("expected media type image/png, got %+v", getStep.ExpectedMedia)
	}
}

func TestAcceptanceTestsIncludesStructuredHTTPStep(t *testing.T) {
	raw := []byte(`{
		"@graph": [
			{
				"@id": "./",
				"subjectOf": [{ "@id": "#acceptance-http" }]
			},
			{
				"@id": "001.jpg",
				"@type": ["File", "ImageObject"],
				"name": "Sample input image",
				"encodingFormat": "image/jpeg"
			},
			{
				"@id": "#expected-zip",
				"@type": ["File", "MediaObject"],
				"encodingFormat": "application/zip"
			},
			{
				"@id": "#acceptance-http",
				"@type": "HowTo",
				"name": "HTTP acceptance test",
				"step": [{ "@id": "#step-http" }]
			},
			{
				"@id": "#step-http",
				"@type": "HowToStep",
				"position": 1,
				"potentialAction": { "@id": "#action-http" }
			},
			{
				"@id": "#action-http",
				"@type": "ConsumeAction",
				"name": "http-request",
				"object": { "@id": "001.jpg" },
				"result": { "@id": "#expected-zip" },
				"additionalProperty": [
					{
						"@id": "#prop-method"
					},
					{
						"@id": "#prop-path"
					},
					{
						"@id": "#prop-accept"
					},
					{
						"@id": "#prop-field"
					}
				]
			},
			{
				"@id": "#prop-method",
				"@type": "PropertyValue",
				"propertyID": "method",
				"value": "POST"
			},
			{
				"@id": "#prop-path",
				"@type": "PropertyValue",
				"propertyID": "path",
				"value": "/v2/models/posenetclas/predict/"
			},
			{
				"@id": "#prop-accept",
				"@type": "PropertyValue",
				"propertyID": "accept",
				"value": "application/zip"
			},
			{
				"@id": "#prop-field",
				"@type": "PropertyValue",
				"propertyID": "formField",
				"value": "data"
			}
		]
	}`)

	crate, err := ParseROCrate(raw)
	if err != nil {
		t.Fatalf("ParseROCrate returned error: %v", err)
	}

	tests, err := crate.AcceptanceTests()
	if err != nil {
		t.Fatalf("AcceptanceTests returned error: %v", err)
	}

	if len(tests) != 1 {
		t.Fatalf("expected 1 acceptance test, got %d", len(tests))
	}

	test := tests[0]
	if len(test.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(test.Steps))
	}

	step := test.Steps[0]
	if step.ParsedCommand == nil || step.ParsedCommand.Kind != stepCommandHTTP {
		t.Fatalf("expected stepCommandHTTP, got %+v", step.ParsedCommand)
	}
	if step.ParsedCommand.HTTPMethod != "POST" {
		t.Fatalf("unexpected HTTP method %q", step.ParsedCommand.HTTPMethod)
	}
	if step.ParsedCommand.HTTPPath != "/v2/models/posenetclas/predict/" {
		t.Fatalf("unexpected HTTP path %q", step.ParsedCommand.HTTPPath)
	}
	if step.ParsedCommand.HTTPFormField != "data" {
		t.Fatalf("unexpected form field %q", step.ParsedCommand.HTTPFormField)
	}
	if len(step.ExpectedMedia) != 1 || step.ExpectedMedia[0] != "application/zip" {
		t.Fatalf("unexpected expected media %+v", step.ExpectedMedia)
	}
	if step.Command != "service http POST /v2/models/posenetclas/predict/" {
		t.Fatalf("unexpected synthetic command %q", step.Command)
	}
}
