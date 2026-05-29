package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates/main.tmpl assets/favicon.svg
var templateFS embed.FS

// RequestInfo structure is used to store the detailed information of captured HTTP requests.
type RequestInfo struct {
	ID         int
	Method     string
	Path       string
	Proto      string
	Headers    http.Header
	Body       string
	Timestamp  time.Time
	RemoteAddr string
}

// PageData structure is used to pass data to the main template.
type PageData struct {
	AllRequests     []RequestInfo
	SelectedRequest *RequestInfo // Using a pointer so that it can be nil when no request is selected
	PathFilter      string
}

// Version information will be set during build
var Version = "dev"

// Constants for error messages
const (
	InternalServerError = "Internal Server Error"
)

// Global store for our requests
// We use global variables to store requests.
var (
	// requestsStore is used to quickly access requests by ID.
	requestsStore = make(map[int]RequestInfo)
	// requestIDs maintains the IDs of all requests in chronological order, used to implement a first-in-first-out (FIFO) limit policy.
	requestIDs []int
	// Use a read-write mutex to protect concurrent access to shared resources.
	mutex = &sync.RWMutex{}
	// nextID is used to generate a unique ID for each new request.
	nextID = 0
	// maxRequests stores the maximum number of requests allowed to be saved.
	maxRequests int
	// cliMode indicates whether to print requests to command line
	cliMode bool
)

// main function is the entry point of the program.
// It sets up routes and starts the HTTP server.
func main() {
	// Define command-line flags to specify port and maximum number of requests.
	port := flag.Int("port", 8282, "Port for the server to listen on")
	listen := flag.String("listen", "127.0.0.1", "Address to listen on")
	max := flag.Int("max", 100, "Maximum number of requests to store")
	cli := flag.Bool("cli", false, "Print requests to command line")
	flag.Parse()

	// Assign the value of command-line parameters to global variables.
	maxRequests = *max
	cliMode = *cli

	// Register the handler function as the handler for all requests.
	http.HandleFunc("/", handler)

	// Build a listening address using the specified address and port.
	addr := fmt.Sprintf("%s:%d", *listen, *port)

	// Start the server and listen on the specified port.
	fmt.Printf("Server starting on port %d...\n", *port)
	fmt.Printf("Maximum requests to store: %d\n", maxRequests)
	fmt.Printf("Send any HTTP request to http://%s:%d/some/path\n", *listen, *port)
	fmt.Printf("View captured requests at http://%s:%d/\n", *listen, *port)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

// handler is the central router for all incoming requests.
// It decides whether to capture the request or display the main panel based on the URL path.
func handler(w http.ResponseWriter, r *http.Request) {
	// Serve the embedded favicon and keep it out of the capture list.
	if r.URL.Path == "/favicon.ico" || r.URL.Path == "/favicon.svg" {
		faviconHandler(w)
		return
	}

	// API endpoint to get requests as JSON for AJAX updates
	if r.URL.Path == "/api/requests" {
		apiRequestsHandler(w, r)
		return
	}

	if r.URL.Path == "/api/requests/clear" {
		clearRequestsHandler(w, r)
		return
	}

	// If it's the root path, display the main panel.
	if r.URL.Path == "/" {
		mainPageHandler(w, r)
		return
	}
	// Otherwise, capture the request.
	captureRequestHandler(w, r)
}

// faviconHandler serves the embedded favicon asset.
func faviconHandler(w http.ResponseWriter) {
	faviconContent, err := templateFS.ReadFile("assets/favicon.svg")
	if err != nil {
		http.Error(w, InternalServerError, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(faviconContent)
}

// PaginatedResponse represents a paginated API response
type PaginatedResponse struct {
	Requests   []RequestInfo `json:"requests"`
	Page       int           `json:"page"`
	Limit      int           `json:"limit"`
	Total      int           `json:"total"`
	TotalPages int           `json:"total_pages"`
	HasNext    bool          `json:"has_next"`
	HasPrev    bool          `json:"has_prev"`
}

// parsePathFilter extracts the path filter from request query parameters.
func parsePathFilter(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("path"))
}

// parsePaginationParams extracts and validates pagination parameters from request
func parsePaginationParams(r *http.Request) (page, limit int) {
	page = 1
	limit = 20 // Default limit

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 { // Max limit of 100
			limit = l
		}
	}

	return page, limit
}

// getFilteredRequests returns requests in reverse chronological order and filters by path when provided.
func getFilteredRequests(pathFilter string) []RequestInfo {
	requests := make([]RequestInfo, 0, len(requestIDs))
	for i := len(requestIDs) - 1; i >= 0; i-- {
		id := requestIDs[i]
		req := requestsStore[id]
		if pathFilter != "" && !strings.Contains(req.Path, pathFilter) {
			continue
		}
		requests = append(requests, req)
	}

	return requests
}

// getRequestsPage returns a paginated slice of requests
func getRequestsPage(requests []RequestInfo, page, limit int) []RequestInfo {
	total := len(requests)
	if total == 0 {
		return make([]RequestInfo, 0)
	}

	offset := (page - 1) * limit
	start := offset
	end := offset + limit
	if end > total {
		end = total
	}

	return requests[start:end]
}

// apiRequestsHandler handles AJAX requests for getting the list of requests with pagination support
func apiRequestsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse pagination parameters
	page, limit := parsePaginationParams(r)
	pathFilter := parsePathFilter(r)

	mutex.RLock()
	filteredRequests := getFilteredRequests(pathFilter)
	total := len(filteredRequests)

	// Calculate pagination
	totalPages := (total + limit - 1) / limit // Ceiling division
	if totalPages == 0 {
		totalPages = 1
	}

	// Validate page number
	if page > totalPages {
		page = totalPages
	}

	// Get paginated requests
	requests := getRequestsPage(filteredRequests, page, limit)

	mutex.RUnlock()

	// Create paginated response
	response := PaginatedResponse{
		Requests:   requests,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
		HasNext:    page < totalPages,
		HasPrev:    page > 1,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode requests", http.StatusInternalServerError)
		return
	}
}

// clearRequestsHandler clears all captured requests.
func clearRequestsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	mutex.Lock()
	requestsStore = make(map[int]RequestInfo)
	requestIDs = nil
	mutex.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// captureRequestHandler captures the details of the incoming request and stores it.
func captureRequestHandler(w http.ResponseWriter, r *http.Request) {
	// Read the request body.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Could not read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	mutex.Lock()
	id := nextID
	nextID++
	mutex.Unlock()

	// Create a new RequestInfo instance.
	reqInfo := RequestInfo{
		ID:         id,
		Method:     r.Method,
		Path:       r.URL.Path,
		Proto:      r.Proto,
		Headers:    r.Header,
		Body:       string(bodyBytes),
		Timestamp:  time.Now(),
		RemoteAddr: r.RemoteAddr,
	}

	// Lock to safely update the storage.
	mutex.Lock()
	requestsStore[id] = reqInfo
	requestIDs = append(requestIDs, id) // Add the ID of the new request to the end of the list.

	// If the number of requests exceeds the maximum limit, delete the oldest one.
	if len(requestIDs) > maxRequests {
		oldestID := requestIDs[0]       // Get the oldest ID.
		delete(requestsStore, oldestID) // Delete it from the map.
		requestIDs = requestIDs[1:]     // Remove from the beginning of the slice.
	}
	mutex.Unlock()

	// Respond to the client, informing that the request has been captured.
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Request captured successfully. View at http://%s/?view_id=%d", r.Host, id)

	// Print request details to CLI if in CLI mode
	if cliMode {
		printRequestToCLI(reqInfo)
	}
}

// mainPageHandler prepares data and renders the main page with a left-right layout.
func mainPageHandler(w http.ResponseWriter, r *http.Request) {
	mutex.RLock()
	pathFilter := parsePathFilter(r)
	requests := getFilteredRequests(pathFilter)
	mutex.RUnlock()

	var selectedReq *RequestInfo

	// Check the 'view_id' parameter to determine which request to display.
	idStr := r.URL.Query().Get("view_id")
	if idStr != "" {
		id, err := strconv.Atoi(idStr)
		if err == nil {
			mutex.RLock()
			// We need to look up directly from the map, as the `requests` list may not contain all historical requests.
			req, ok := requestsStore[id]
			mutex.RUnlock()
			if ok {
				selectedReq = &req
			}
		}
	}

	// If no request is selected and there are requests, default to showing the newest one.
	if selectedReq == nil && len(requests) > 0 {
		selectedReq = &requests[0]
	}

	pageData := PageData{
		AllRequests:     requests,
		SelectedRequest: selectedReq,
		PathFilter:      pathFilter,
	}

	renderTemplate(w, "main", pageData)
}

// renderTemplate is a helper function for parsing and executing HTML templates.
func renderTemplate(w http.ResponseWriter, tmplName string, data interface{}) {
	// Read the template from the embedded file system
	tmplContent, err := templateFS.ReadFile("templates/main.tmpl")
	if err != nil {
		log.Printf("Error reading template file: %v", err)
		http.Error(w, InternalServerError, http.StatusInternalServerError)
		return
	}

	tmpl, err := template.New(tmplName).Funcs(template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"isCurrent": func(currentID int, selectedID *RequestInfo) bool {
			if selectedID == nil {
				return false
			}
			return currentID == selectedID.ID
		},
		"prettyPrintJson": func(body string) template.HTML {
			var prettyJSON bytes.Buffer
			if err := json.Indent(&prettyJSON, []byte(body), "", "  "); err == nil {
				return template.HTML(prettyJSON.String())
			}
			return template.HTML(template.HTMLEscapeString(body))
		},
	}).Parse(string(tmplContent))

	if err != nil {
		log.Printf("Error parsing template %s: %v", tmplName, err)
		http.Error(w, InternalServerError, http.StatusInternalServerError)
		return
	}

	buf := &bytes.Buffer{}
	err = tmpl.Execute(buf, data)
	if err != nil {
		log.Printf("Error executing template %s: %v", tmplName, err)
		http.Error(w, InternalServerError, http.StatusInternalServerError)
		return
	}

	buf.WriteTo(w)
}

// printRequestToCLI prints request information to command line in a beautiful format
func printRequestToCLI(reqInfo RequestInfo) {
	fmt.Printf("\n%s\n", strings.Repeat("=", 80))
	fmt.Printf("📨 Request #%d captured at %s\n", reqInfo.ID, reqInfo.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("%s\n", strings.Repeat("-", 80))

	// Request line
	fmt.Printf("🔹 %s %s %s\n", reqInfo.Method, reqInfo.Path, reqInfo.Proto)
	fmt.Printf("🔹 Remote Address: %s\n", reqInfo.RemoteAddr)

	// Headers
	if len(reqInfo.Headers) > 0 {
		fmt.Printf("\n📋 Headers:\n")
		for name, values := range reqInfo.Headers {
			for _, value := range values {
				fmt.Printf("   %s: %s\n", name, value)
			}
		}
	}

	// Body
	if reqInfo.Body != "" {
		fmt.Printf("\n📄 Body:\n")
		// Try to pretty print JSON
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, []byte(reqInfo.Body), "   ", "  "); err == nil {
			fmt.Printf("   %s\n", prettyJSON.String())
		} else {
			// Not JSON, print as is with indentation
			lines := strings.Split(reqInfo.Body, "\n")
			for _, line := range lines {
				fmt.Printf("   %s\n", line)
			}
		}
	} else {
		fmt.Printf("\n📄 Body: (empty)\n")
	}

	fmt.Printf("%s\n", strings.Repeat("=", 80))

	// Force flush stdout to ensure immediate output
	os.Stdout.Sync()
}
