package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// createFinalTemplate processes a raw template, injecting payload data and asset URLs.
func createFinalTemplate(ctx context.Context, rawTemplate json.RawMessage, payload map[string]interface{}, bgImageURL string, project Project) (json.RawMessage, error) {
	var templateData map[string]interface{}
	if err := json.Unmarshal(rawTemplate, &templateData); err != nil {
		return nil, err
	}

	// This logic is from your archive, preserved.
	if layers, ok := templateData["layers"].([]interface{}); ok {
		if bgImageURL != "" && len(layers) > 0 {
			if firstLayer, ok := layers[0].(map[string]interface{}); ok {
				firstLayer["file"] = bgImageURL
			}
		}
	}

	// Recursively process all strings in the template
	if err := traverseAndProcess(ctx, templateData, payload, project); err != nil {
		return nil, err
	}
	return json.Marshal(templateData)
}

// traverseAndProcess recursively walks through the template data (maps and slices)
// and applies processing functions to all string values.
func traverseAndProcess(ctx context.Context, data interface{}, payload map[string]interface{}, project Project) error {
	var processFunc func(d interface{}) error
	processFunc = func(d interface{}) error {
		switch v := d.(type) {
		case map[string]interface{}:
			for key, val := range v {
				if strVal, ok := val.(string); ok {

					isAssetField := key == "file" || key == "font_file"
					finalValue := strVal

					// 1. Check for and process ONLY asset placeholders.
					if strings.Contains(finalValue, "_ASSETS}}") {
						assetPath := strings.ReplaceAll(finalValue, "{{SYSTEM_ASSETS}}", "system")
						assetPath = strings.ReplaceAll(assetPath, "{{ORGANIZATION_ASSETS}}", fmt.Sprintf("organizations/%s/assets", project.OrganizationKey))
						assetPath = strings.ReplaceAll(assetPath, "{{PROJECT_ASSETS}}", fmt.Sprintf("organizations/%s/%s/assets", project.OrganizationKey, project.ProjectKey))

						// Only pre-sign if it's a file/font_file field.
						if isAssetField {
							url, err := getPresignedURL(ctx, s3Client.InternalPresigner, assetPath)
							if err != nil {
								return err
							}
							finalValue = url
						} else {
							finalValue = assetPath // Should not happen, but safe.
						}
					}

					// 2. Separately, check for and process payload placeholders.
					// This block is now independent of the asset pre-signing logic.
					if strings.Contains(finalValue, "{{.") {
						processedVal, err := processPayloadTemplateString(finalValue, payload)
						if err != nil {
							return err
						}
						finalValue = processedVal
					}

					v[key] = finalValue
				} else {
					// Recurse into nested data structures
					if err := processFunc(val); err != nil {
						return err
					}
				}
			}
		case []interface{}:
			for _, item := range v {
				if err := processFunc(item); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return processFunc(data)
}

// processPayloadTemplateString executes a string as a Go template with the user's payload.
func processPayloadTemplateString(text string, payload map[string]interface{}) (string, error) {
	if !strings.Contains(text, "{{.") {
		return text, nil
	}
	tmpl, err := template.New("").Parse(text)
	if err != nil {
		return "", fmt.Errorf("failed to parse template string '%s': %w", text, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, payload); err != nil {
		return "", fmt.Errorf("failed to execute template string '%s': %w", text, err)
	}
	return buf.String(), nil
}
