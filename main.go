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
	ProjectID  string
}

// Project structure represents a project workspace
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// PageData structure is used to pass data to the main template.
type PageData struct {
	AllRequests     []RequestInfo
	SelectedRequest *RequestInfo // Using a pointer so that it can be nil when no request is selected
	Projects        []Project
	CurrentProject  *Project
}

// Version information will be set during build
var Version = "dev"

// Constants for error messages
const (
	InternalServerError = "Internal Server Error"
)

// Global store for our requests and projects
// We use global variables to store requests and projects.
var (
	// requestsStore is used to quickly access requests by ID.
	requestsStore = make(map[int]RequestInfo)
	// requestIDs maintains the IDs of all requests in chronological order, used to implement a first-in-first-out (FIFO) limit policy.
	requestIDs []int
	// projectsStore stores all projects by their ID
	projectsStore = make(map[string]Project)
	// projectRequests maps project ID to request IDs for that project
	projectRequests = make(map[string][]int)
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

	// Initialize default project
	initializeDefaultProject()

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

// initializeDefaultProject creates a default project if none exists
func initializeDefaultProject() {
	mutex.Lock()
	defer mutex.Unlock()
	
	defaultProject := Project{
		ID:          "default",
		Name:        "Default Project",
		Description: "Default project for all requests",
		CreatedAt:   time.Now(),
	}
	
	projectsStore["default"] = defaultProject
	projectRequests["default"] = make([]int, 0)
}

// handler is the central router for all incoming requests.
// It decides whether to capture the request or display the main panel based on the URL path.
func handler(w http.ResponseWriter, r *http.Request) {
	// Ignore requests for browser icons, directly return 204 No Content, so they won't be recorded.
	if r.URL.Path == "/favicon.ico" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// API endpoint to get requests as JSON for AJAX updates
	if r.URL.Path == "/api/requests" {
		apiRequestsHandler(w, r)
		return
	}

	// API endpoint to get projects
	if r.URL.Path == "/api/projects" {
		apiProjectsHandler(w, r)
		return
	}

	// API endpoint to create a new project
	if r.URL.Path == "/api/projects/create" && r.Method == "POST" {
		createProjectHandler(w, r)
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

// getCurrentProject gets the current project from the request or returns default
func getCurrentProject(r *http.Request) string {
	projectID := r.URL.Query().Get("project")
	if projectID == "" {
		projectID = "default"
	}
	
	mutex.RLock()
	_, exists := projectsStore[projectID]
	mutex.RUnlock()
	
	if !exists {
		return "default"
	}
	
	return projectID
}

// apiProjectsHandler handles requests for getting project list
func apiProjectsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	mutex.RLock()
	projects := make([]Project, 0, len(projectsStore))
	for _, project := range projectsStore {
		projects = append(projects, project)
	}
	mutex.RUnlock()
	
	if err := json.NewEncoder(w).Encode(projects); err != nil {
		http.Error(w, "Failed to encode projects", http.StatusInternalServerError)
		return
	}
}

// CreateProjectRequest represents the request body for creating a new project
type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// createProjectHandler handles creating new projects
func createProjectHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	if req.Name == "" {
		http.Error(w, "Project name is required", http.StatusBadRequest)
		return
	}
	
	// Generate a simple ID based on name
	projectID := strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))
	projectID = fmt.Sprintf("%s-%d", projectID, time.Now().Unix())
	
	mutex.Lock()
	project := Project{
		ID:          projectID,
		Name:        req.Name,
		Description: req.Description,
		CreatedAt:   time.Now(),
	}
	projectsStore[projectID] = project
	projectRequests[projectID] = make([]int, 0)
	mutex.Unlock()
	
	if err := json.NewEncoder(w).Encode(project); err != nil {
		http.Error(w, "Failed to encode project", http.StatusInternalServerError)
		return
	}
}
type PaginatedResponse struct {
	Requests   []RequestInfo `json:"requests"`
	Page       int           `json:"page"`
	Limit      int           `json:"limit"`
	Total      int           `json:"total"`
	TotalPages int           `json:"total_pages"`
	HasNext    bool          `json:"has_next"`
	HasPrev    bool          `json:"has_prev"`
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

// getRequestsPage returns a paginated slice of requests for a specific project
func getRequestsPage(page, limit, total int, projectID string) []RequestInfo {
	if total == 0 {
		return make([]RequestInfo, 0)
	}

	mutex.RLock()
	projectReqIDs, exists := projectRequests[projectID]
	mutex.RUnlock()
	
	if !exists {
		return make([]RequestInfo, 0)
	}

	// Calculate pagination
	offset := (page - 1) * limit
	start := offset
	end := offset + limit
	if end > len(projectReqIDs) {
		end = len(projectReqIDs)
	}

	requests := make([]RequestInfo, end-start)
	for i := start; i < end; i++ {
		id := projectReqIDs[len(projectReqIDs)-1-i] // Reverse order to show newest first
		requests[i-start] = requestsStore[id]
	}

	return requests
}

// apiRequestsHandler handles AJAX requests for getting the list of requests with pagination support
func apiRequestsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse pagination parameters
	page, limit := parsePaginationParams(r)
	
	// Get current project
	projectID := getCurrentProject(r)

	mutex.RLock()
	projectReqIDs, exists := projectRequests[projectID]
	if !exists {
		mutex.RUnlock()
		// Project doesn't exist, return empty result
		response := PaginatedResponse{
			Requests:   make([]RequestInfo, 0),
			Page:       1,
			Limit:      limit,
			Total:      0,
			TotalPages: 1,
			HasNext:    false,
			HasPrev:    false,
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	total := len(projectReqIDs)

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
	requests := getRequestsPage(page, limit, total, projectID)

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

// captureRequestHandler captures the details of the incoming request and stores it.
func captureRequestHandler(w http.ResponseWriter, r *http.Request) {
	// Read the request body.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Could not read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	// Get current project from query parameter, default to "default"
	projectID := getCurrentProject(r)

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
		ProjectID:  projectID,
	}

	// Lock to safely update the storage.
	mutex.Lock()
	requestsStore[id] = reqInfo
	requestIDs = append(requestIDs, id) // Add to global list
	
	// Add to project-specific list
	projectRequests[projectID] = append(projectRequests[projectID], id)

	// If the number of requests in this project exceeds the maximum limit, delete the oldest one.
	if len(projectRequests[projectID]) > maxRequests {
		oldestID := projectRequests[projectID][0]       // Get the oldest ID for this project.
		delete(requestsStore, oldestID) // Delete it from the global map.
		projectRequests[projectID] = projectRequests[projectID][1:]     // Remove from the beginning of the project slice.
		
		// Also remove from global requestIDs list
		for i, globalID := range requestIDs {
			if globalID == oldestID {
				requestIDs = append(requestIDs[:i], requestIDs[i+1:]...)
				break
			}
		}
	}
	mutex.Unlock()

	// Respond to the client, informing that the request has been captured.
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Request captured successfully in project '%s'. View at http://%s/?project=%s&view_id=%d", projectID, r.Host, projectID, id)

	// Print request details to CLI if in CLI mode
	if cliMode {
		printRequestToCLI(reqInfo)
	}
}

// mainPageHandler prepares data and renders the main page with a left-right layout.
func mainPageHandler(w http.ResponseWriter, r *http.Request) {
	// Get current project
	projectID := getCurrentProject(r)
	
	mutex.RLock()
	// Get projects list
	projects := make([]Project, 0, len(projectsStore))
	for _, project := range projectsStore {
		projects = append(projects, project)
	}
	
	// Get current project details
	var currentProject *Project
	if proj, exists := projectsStore[projectID]; exists {
		currentProject = &proj
	}
	
	// Create a list of requests for the current project based on the ordered ID list.
	// To have the newest requests shown at the top, we traverse the ID list in reverse order.
	projectReqIDs, exists := projectRequests[projectID]
	requests := make([]RequestInfo, 0)
	if exists {
		requests = make([]RequestInfo, len(projectReqIDs))
		for i := 0; i < len(projectReqIDs); i++ {
			id := projectReqIDs[len(projectReqIDs)-1-i]
			requests[i] = requestsStore[id]
		}
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
			if ok && req.ProjectID == projectID { // Ensure request belongs to current project
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
		Projects:        projects,
		CurrentProject:  currentProject,
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
