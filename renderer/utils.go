package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

func downloadAsset(assetUrl, tmpDir string) (string, error) {
	parsedURL, _ := url.Parse(assetUrl)
	localPath := filepath.Join(tmpDir, filepath.Base(parsedURL.Path))
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}
	return localPath, downloadFileToPath(assetUrl, localPath)
}

func getPNGDimensions(imagePath string) (int, int, error) {
	cmd := exec.Command("magick", "identify", "-format", "%w %h", imagePath)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0, 0, err
	}
	parts := strings.Fields(out.String())
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("bad identify output")
	}
	width, _ := strconv.Atoi(parts[0])
	height, _ := strconv.Atoi(parts[1])
	return width, height, nil
}

func generateEmptyPNG(path string) error {
	return runCommand("magick", "-size", "1x1", "xc:transparent", path)
}

func downloadFileToPath(rawURL, path string) error {
	resp, err := http.Get(rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s for URL: %s", resp.Status, rawURL)
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func downloadFileFromURL(rawURL, dir, prefix string) (string, error) {
	resp, err := http.Get(rawURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s for URL: %s", resp.Status, rawURL)
	}
	parsedURL, _ := url.Parse(rawURL)
	tmpfile, err := os.CreateTemp(dir, prefix+"_*"+filepath.Ext(parsedURL.Path))
	if err != nil {
		return "", err
	}
	defer tmpfile.Close()
	_, err = io.Copy(tmpfile, resp.Body)
	return tmpfile.Name(), err
}

func processTemplateString(text string, payload map[string]interface{}) (string, error) {
	if !strings.Contains(text, "{{") {
		return text, nil
	}
	tmpl, err := template.New("").Parse(text)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, payload); err != nil {
		return "", err
	}
	return buf.String(), nil
}
