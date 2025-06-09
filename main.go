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
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates/main.tmpl
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
}

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
	port := flag.Int("port", 8080, "Port for the server to listen on")
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
	// Ignore requests for browser icons, directly return 204 No Content, so they won't be recorded.
	if r.URL.Path == "/favicon.ico" {
		w.WriteHeader(http.StatusNoContent)
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
	// Create a list of requests based on the ordered ID list.
	// To have the newest requests shown at the top, we traverse the ID list in reverse order.
	requests := make([]RequestInfo, len(requestIDs))
	for i := 0; i < len(requestIDs); i++ {
		id := requestIDs[len(requestIDs)-1-i]
		requests[i] = requestsStore[id]
	}
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
	}

	renderTemplate(w, "main", pageData)
}

// renderTemplate is a helper function for parsing and executing HTML templates.
func renderTemplate(w http.ResponseWriter, tmplName string, data interface{}) {
	// Read the template from the embedded file system
	tmplContent, err := templateFS.ReadFile("templates/main.tmpl")
	if err != nil {
		log.Printf("Error reading template file: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	buf := &bytes.Buffer{}
	err = tmpl.Execute(buf, data)
	if err != nil {
		log.Printf("Error executing template %s: %v", tmplName, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
}
