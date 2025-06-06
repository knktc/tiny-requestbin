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
	"sync"
	"time"
)

//go:embed templates/main.tmpl
var templateFS embed.FS

// RequestInfo 结构用于存储捕获到的 HTTP 请求的详细信息。
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

// PageData 结构用于向主模板传递数据。
type PageData struct {
	AllRequests     []RequestInfo
	SelectedRequest *RequestInfo // 使用指针，以便在未选择任何请求时可以为 nil
}

// Global store for our requests
// 我们使用一个全局变量来存储请求。
var (
	// requestsStore 用于通过 ID 快速访问请求。
	requestsStore = make(map[int]RequestInfo)
	// requestIDs 按时间顺序维护所有请求的 ID，用于实现先进先出（FIFO）的限制策略。
	requestIDs []int
	// 使用读写互斥锁来保护对共享资源的并发访问。
	mutex = &sync.RWMutex{}
	// nextID 用于为每个新请求生成一个唯一的 ID。
	nextID = 0
	// maxRequests 存储允许保存的最大请求数量。
	maxRequests int
)

// main 函数是程序的入口点。
// 它设置路由并启动 HTTP 服务器。
func main() {
	// 定义命令行标志来指定端口和最大请求数。
	port := flag.String("port", "8080", "Port for the server to listen on")
	max := flag.Int("max", 100, "Maximum number of requests to store")
	flag.Parse()

	// 将命令行参数的值赋给全局变量。
	maxRequests = *max

	// 将 handler 函数注册为所有请求的处理器。
	http.HandleFunc("/", handler)

	// 使用指定的端口构建监听地址。
	addr := ":" + *port

	// 启动服务器并监听指定的端口。
	fmt.Printf("Server starting on port %s...\n", *port)
	fmt.Printf("Maximum requests to store: %d\n", maxRequests)
	fmt.Printf("Send any HTTP request to http://localhost:%s/some/path\n", *port)
	fmt.Printf("View captured requests at http://localhost:%s/\n", *port)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

// handler 是所有传入请求的中心路由器。
// 它根据 URL 路径决定是捕获请求还是显示主页面板。
func handler(w http.ResponseWriter, r *http.Request) {
	// 忽略对浏览器图标的请求，直接返回204 No Content，这样就不会被记录。
	if r.URL.Path == "/favicon.ico" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// 如果是根路径，则显示主页面板。
	if r.URL.Path == "/" {
		mainPageHandler(w, r)
		return
	}
	// 否则，捕获该请求。
	captureRequestHandler(w, r)
}

// captureRequestHandler 捕获传入请求的详细信息并将其存储起来。
func captureRequestHandler(w http.ResponseWriter, r *http.Request) {
	// 读取请求正文。
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

	// 创建一个新的 RequestInfo 实例。
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

	// 加锁以安全地更新存储。
	mutex.Lock()
	requestsStore[id] = reqInfo
	requestIDs = append(requestIDs, id) // 将新请求的 ID 添加到列表末尾。

	// 如果请求数量超过了最大限制，则删除最旧的一个。
	if len(requestIDs) > maxRequests {
		oldestID := requestIDs[0]       // 获取最旧的 ID。
		delete(requestsStore, oldestID) // 从 map 中删除。
		requestIDs = requestIDs[1:]     // 从 slice 的开头移除。
	}
	mutex.Unlock()

	// 响应客户端，告知请求已被捕获。
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Request captured successfully. View at http://%s/?view_id=%d", r.Host, id)
}

// mainPageHandler 准备数据并渲染左右布局的主页。
func mainPageHandler(w http.ResponseWriter, r *http.Request) {
	mutex.RLock()
	// 基于有序的 ID 列表创建请求列表。
	// 为了让最新的请求显示在最前面，我们倒序遍历 ID 列表。
	requests := make([]RequestInfo, len(requestIDs))
	for i := 0; i < len(requestIDs); i++ {
		id := requestIDs[len(requestIDs)-1-i]
		requests[i] = requestsStore[id]
	}
	mutex.RUnlock()

	var selectedReq *RequestInfo

	// 检查 'view_id' 参数以确定要显示的请求。
	idStr := r.URL.Query().Get("view_id")
	if idStr != "" {
		id, err := strconv.Atoi(idStr)
		if err == nil {
			mutex.RLock()
			// 我们需要直接从 map 中查找，因为 `requests` 列表可能不包含所有历史请求。
			req, ok := requestsStore[id]
			mutex.RUnlock()
			if ok {
				selectedReq = &req
			}
		}
	}

	// 如果没有选择任何请求且存在请求，则默认显示最新的一个。
	if selectedReq == nil && len(requests) > 0 {
		selectedReq = &requests[0]
	}

	pageData := PageData{
		AllRequests:     requests,
		SelectedRequest: selectedReq,
	}

	renderTemplate(w, "main", pageData)
}

// renderTemplate 是一个辅助函数，用于解析和执行 HTML 模板。
func renderTemplate(w http.ResponseWriter, tmplName string, data interface{}) {
	// 从嵌入的文件系统中读取模板
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
