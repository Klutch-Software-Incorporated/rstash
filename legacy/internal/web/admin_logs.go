package web

import (
	"bufio"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type logsHandler struct {
	deps *UIDeps
}

// LogsHandler returns handler methods for the admin logs page.
func LogsHandler(deps *UIDeps) *logsHandler {
	return &logsHandler{deps: deps}
}

type adminLogsContent struct {
	LogFile    string
	Lines      []string
	MaxLines   int
	LevelFilter string
	HasLogFile bool
}

// ShowLogs handles GET /admin/logs — displays recent log file entries.
func (h *logsHandler) ShowLogs(w http.ResponseWriter, r *http.Request) {
	maxLines := 200
	if n, err := strconv.Atoi(r.URL.Query().Get("lines")); err == nil && n > 0 && n <= 5000 {
		maxLines = n
	}
	levelFilter := r.URL.Query().Get("level")

	content := &adminLogsContent{
		LogFile:     h.deps.LogFile,
		MaxLines:    maxLines,
		LevelFilter: levelFilter,
		HasLogFile:  h.deps.LogFile != "",
	}

	if h.deps.LogFile != "" {
		lines, _ := tailFile(h.deps.LogFile, maxLines)
		if levelFilter != "" {
			lines = filterByLevel(lines, levelFilter)
		}
		content.Lines = lines
	}

	h.deps.Renderer.Render(w, "admin_logs", h.deps.adminPageData(w, r, "Logs — Admin", "logs", content))
}

// tailFile reads the last maxLines lines from a file.
func tailFile(path string, maxLines int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var allLines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(allLines) > maxLines {
		allLines = allLines[len(allLines)-maxLines:]
	}
	return allLines, nil
}

// filterByLevel filters log lines to only include those matching the given level.
func filterByLevel(lines []string, level string) []string {
	target := "level=" + strings.ToUpper(level)
	var filtered []string
	for _, line := range lines {
		if strings.Contains(line, target) {
			filtered = append(filtered, line)
		}
	}
	return filtered
}
