// main.go
//
// Gmail API Emulator for Docker deployment
// Serves transformed Enron data as Gmail API
// Version: 2.0
// Last Updated: 2025-07-13
//
// Carson Sweet assisted by Claude AI
// https://www.carsonsweet.com

// main.go
//
// Gmail API Emulator for Docker deployment
// Serves transformed Enron data as Gmail API
// Version: 2.3 - Added endpoint to list all email addresses in dataset
// Last Updated: 2025-07-13

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

// Gmail API structures
type GmailMessage struct {
	Id           string       `json:"id"`
	ThreadId     string       `json:"threadId"`
	LabelIds     []string     `json:"labelIds"`
	Snippet      string       `json:"snippet"`
	HistoryId    string       `json:"historyId"`
	InternalDate string       `json:"internalDate"`
	SizeEstimate int          `json:"sizeEstimate"`
	Payload      *MessagePart `json:"payload"`
}

type MessagePart struct {
	PartId   string        `json:"partId,omitempty"`
	MimeType string        `json:"mimeType"`
	Filename string        `json:"filename,omitempty"`
	Headers  []Header      `json:"headers"`
	Body     *MessageBody  `json:"body,omitempty"`
	Parts    []MessagePart `json:"parts,omitempty"`
}

type MessageBody struct {
	Size int    `json:"size"`
	Data string `json:"data"`
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ListMessagesResponse struct {
	Messages           []MessageRef `json:"messages"`
	NextPageToken      string       `json:"nextPageToken,omitempty"`
	ResultSizeEstimate int          `json:"resultSizeEstimate"`
}

type MessageRef struct {
	Id       string `json:"id"`
	ThreadId string `json:"threadId"`
}

type UserProfile struct {
	EmailAddress  string `json:"emailAddress"`
	MessagesTotal int    `json:"messagesTotal"`
	ThreadsTotal  int    `json:"threadsTotal"`
	HistoryId     string `json:"historyId"`
}

type Label struct {
	Id                    string `json:"id"`
	Name                  string `json:"name"`
	MessageListVisibility string `json:"messageListVisibility"`
	LabelListVisibility   string `json:"labelListVisibility"`
	Type                  string `json:"type"`
}

// New structure for user list endpoint
type UserInfo struct {
	Email        string `json:"email"`
	Name         string `json:"name,omitempty"`
	MessageCount int    `json:"messageCount"`
	Type         string `json:"type"` // "primary", "contact", "service"
}

// GmailEmulator serves Gmail API responses
type GmailEmulator struct {
	messages       map[string]*GmailMessage
	messageList    []MessageRef
	messagesByDate []*GmailMessage // Sorted by date
	userEmail      string
	dataPath       string
	requestLog     []RequestLog
	userList       []UserInfo // New field for caching user list
}

type RequestLog struct {
	Method    string
	Path      string
	Query     string
	Timestamp time.Time
}

func NewGmailEmulator(dataPath, userEmail string) (*GmailEmulator, error) {
	emulator := &GmailEmulator{
		messages:   make(map[string]*GmailMessage),
		userEmail:  userEmail,
		dataPath:   dataPath,
		requestLog: []RequestLog{},
		userList:   []UserInfo{},
	}

	// Load messages
	messagesPath := filepath.Join(dataPath, "gmail_messages.json")
	messagesData, err := ioutil.ReadFile(messagesPath)
	if err != nil {
		return nil, fmt.Errorf("read messages file: %w", err)
	}

	var messageSlice []*GmailMessage
	if err := json.Unmarshal(messagesData, &messageSlice); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}

	// Index messages and build list
	for _, msg := range messageSlice {
		emulator.messages[msg.Id] = msg
		emulator.messageList = append(emulator.messageList, MessageRef{
			Id:       msg.Id,
			ThreadId: msg.ThreadId,
		})
	}

	// Sort by date for query filtering
	emulator.messagesByDate = messageSlice

	// Build user list from messages
	emulator.buildUserList()

	log.Printf("Loaded %d messages from %s", len(emulator.messages), dataPath)
	log.Printf("Found %d unique email addresses in dataset", len(emulator.userList))

	return emulator, nil
}

// New method to build user list from messages
func (e *GmailEmulator) buildUserList() {
	userMap := make(map[string]*UserInfo)

	// Add the primary user
	userMap[e.userEmail] = &UserInfo{
		Email:        e.userEmail,
		Name:         "You",
		MessageCount: 0,
		Type:         "primary",
	}

	// Scan all messages for email addresses
	for _, msg := range e.messages {
		if msg.Payload == nil {
			continue
		}

		// Check From header
		fromEmail, fromName := e.extractEmailAndName(e.getHeader(msg, "From"))
		if fromEmail != "" {
			if info, exists := userMap[fromEmail]; exists {
				info.MessageCount++
			} else {
				userMap[fromEmail] = &UserInfo{
					Email:        fromEmail,
					Name:         fromName,
					MessageCount: 1,
					Type:         e.determineUserType(fromEmail),
				}
			}
		}

		// Check To headers
		toHeader := e.getHeader(msg, "To")
		for _, recipient := range strings.Split(toHeader, ",") {
			email, name := e.extractEmailAndName(strings.TrimSpace(recipient))
			if email != "" {
				if info, exists := userMap[email]; exists {
					info.MessageCount++
					if info.Name == "" && name != "" {
						info.Name = name
					}
				} else {
					userMap[email] = &UserInfo{
						Email:        email,
						Name:         name,
						MessageCount: 1,
						Type:         e.determineUserType(email),
					}
				}
			}
		}

		// Check CC headers
		ccHeader := e.getHeader(msg, "Cc")
		if ccHeader != "" {
			for _, recipient := range strings.Split(ccHeader, ",") {
				email, name := e.extractEmailAndName(strings.TrimSpace(recipient))
				if email != "" {
					if info, exists := userMap[email]; exists {
						info.MessageCount++
						if info.Name == "" && name != "" {
							info.Name = name
						}
					} else {
						userMap[email] = &UserInfo{
							Email:        email,
							Name:         name,
							MessageCount: 1,
							Type:         e.determineUserType(email),
						}
					}
				}
			}
		}
	}

	// Convert map to sorted slice
	for _, user := range userMap {
		e.userList = append(e.userList, *user)
	}

	// Sort by message count (descending) and then by email
	sort.Slice(e.userList, func(i, j int) bool {
		if e.userList[i].MessageCount != e.userList[j].MessageCount {
			return e.userList[i].MessageCount > e.userList[j].MessageCount
		}
		return e.userList[i].Email < e.userList[j].Email
	})
}

// Extract email and name from headers like "John Doe <john@example.com>"
func (e *GmailEmulator) extractEmailAndName(headerValue string) (email, name string) {
	if headerValue == "" {
		return "", ""
	}

	// Handle "Name <email>" format
	if idx := strings.Index(headerValue, "<"); idx >= 0 {
		if endIdx := strings.Index(headerValue[idx:], ">"); endIdx > 0 {
			email = strings.TrimSpace(headerValue[idx+1 : idx+endIdx])
			name = strings.TrimSpace(headerValue[:idx])
			return email, name
		}
	}

	// Plain email address
	if strings.Contains(headerValue, "@") {
		return strings.TrimSpace(headerValue), ""
	}

	return "", ""
}

// Determine user type based on email patterns
func (e *GmailEmulator) determineUserType(email string) string {
	email = strings.ToLower(email)

	if email == strings.ToLower(e.userEmail) {
		return "primary"
	}

	// Service emails
	servicePatterns := []string{
		"no-reply", "noreply", "donotreply",
		"notification", "alert", "system",
		"@github.com", "@linkedin.com", "@united.com",
	}

	for _, pattern := range servicePatterns {
		if strings.Contains(email, pattern) {
			return "service"
		}
	}

	return "contact"
}

// Get header value by name
func (e *GmailEmulator) getHeader(msg *GmailMessage, headerName string) string {
	if msg.Payload == nil {
		return ""
	}

	for _, header := range msg.Payload.Headers {
		if header.Name == headerName {
			return header.Value
		}
	}

	return ""
}

// New endpoint handler for listing users
func (e *GmailEmulator) handleListUsers(w http.ResponseWriter, r *http.Request) {
	e.logRequest(r)

	// Optional query parameters
	typeFilter := r.URL.Query().Get("type")
	limitStr := r.URL.Query().Get("limit")

	limit := 0
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	// Filter users
	filtered := []UserInfo{}
	for _, user := range e.userList {
		if typeFilter == "" || user.Type == typeFilter {
			filtered = append(filtered, user)
		}
	}

	// Apply limit
	if limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}

	response := map[string]interface{}{
		"users":      filtered,
		"totalCount": len(e.userList),
		"metadata": map[string]interface{}{
			"primaryUser": e.userEmail,
			"dataPath":    e.dataPath,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// API Handlers

func (e *GmailEmulator) handleProfile(w http.ResponseWriter, r *http.Request) {
	userId := mux.Vars(r)["userId"]

	if userId != "me" && userId != e.userEmail {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	profile := UserProfile{
		EmailAddress:  e.userEmail,
		MessagesTotal: len(e.messages),
		ThreadsTotal:  e.countThreads(),
		HistoryId:     fmt.Sprintf("%d", time.Now().Unix()),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

func (e *GmailEmulator) handleLabels(w http.ResponseWriter, r *http.Request) {
	labels := []Label{
		{Id: "INBOX", Name: "INBOX", Type: "system"},
		{Id: "SENT", Name: "SENT", Type: "system"},
		{Id: "DRAFT", Name: "DRAFT", Type: "system"},
		{Id: "SPAM", Name: "SPAM", Type: "system"},
		{Id: "TRASH", Name: "TRASH", Type: "system"},
		{Id: "UNREAD", Name: "UNREAD", Type: "system"},
		{Id: "IMPORTANT", Name: "IMPORTANT", Type: "system"},
		{Id: "CATEGORY_PERSONAL", Name: "CATEGORY_PERSONAL", Type: "system"},
		{Id: "CATEGORY_SOCIAL", Name: "CATEGORY_SOCIAL", Type: "system"},
		{Id: "CATEGORY_PROMOTIONS", Name: "CATEGORY_PROMOTIONS", Type: "system"},
		{Id: "CATEGORY_UPDATES", Name: "CATEGORY_UPDATES", Type: "system"},
	}

	response := map[string][]Label{"labels": labels}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (e *GmailEmulator) handleListMessages(w http.ResponseWriter, r *http.Request) {
	e.logRequest(r)

	// Parse query parameters
	q := r.URL.Query().Get("q")
	pageToken := r.URL.Query().Get("pageToken")
	maxResults := r.URL.Query().Get("maxResults")
	labelIds := r.URL.Query().Get("labelIds")

	// Default max results
	limit := 100
	if maxResults != "" {
		if n, err := strconv.Atoi(maxResults); err == nil && n > 0 {
			limit = n
		}
	}

	// Parse page token
	start := 0
	if pageToken != "" {
		if n, err := strconv.Atoi(pageToken); err == nil {
			start = n
		}
	}

	// Filter messages
	filtered := e.filterMessages(q, labelIds)

	// Apply pagination
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	response := ListMessagesResponse{
		Messages:           filtered[start:end],
		ResultSizeEstimate: len(filtered),
	}

	// Set next page token if there are more results
	if end < len(filtered) {
		response.NextPageToken = strconv.Itoa(end)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (e *GmailEmulator) handleGetMessage(w http.ResponseWriter, r *http.Request) {
	e.logRequest(r)

	messageId := mux.Vars(r)["messageId"]
	format := r.URL.Query().Get("format")

	msg, ok := e.messages[messageId]
	if !ok {
		http.Error(w, `{"error": {"code": 404, "message": "Message not found"}}`, http.StatusNotFound)
		return
	}

	// Handle different format requests
	response := msg
	if format == "metadata" {
		// Return without body
		metadataMsg := *msg
		if metadataMsg.Payload != nil {
			metadataPayload := *metadataMsg.Payload
			metadataPayload.Body = nil
			metadataMsg.Payload = &metadataPayload
		}
		response = &metadataMsg
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (e *GmailEmulator) handleBatchGet(w http.ResponseWriter, r *http.Request) {
	e.logRequest(r)

	// Parse request body
	var request struct {
		Ids []string `json:"ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, `{"error": {"code": 400, "message": "Invalid request"}}`, http.StatusBadRequest)
		return
	}

	messages := []GmailMessage{}
	for _, id := range request.Ids {
		if msg, ok := e.messages[id]; ok {
			messages = append(messages, *msg)
		}
	}

	response := map[string][]GmailMessage{"messages": messages}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper methods

func (e *GmailEmulator) filterMessages(query, labelIds string) []MessageRef {
	filtered := []MessageRef{}

	// Parse label IDs
	labels := []string{}
	if labelIds != "" {
		labels = strings.Split(labelIds, ",")
	}

	for _, ref := range e.messageList {
		msg := e.messages[ref.Id]

		// Filter by labels
		if len(labels) > 0 {
			hasLabel := false
			for _, requiredLabel := range labels {
				for _, msgLabel := range msg.LabelIds {
					if msgLabel == requiredLabel {
						hasLabel = true
						break
					}
				}
				if hasLabel {
					break
				}
			}
			if !hasLabel {
				continue
			}
		}

		// Filter by query
		if query != "" && !e.matchesQuery(msg, query) {
			continue
		}

		filtered = append(filtered, ref)
	}

	return filtered
}

func (e *GmailEmulator) matchesQuery(msg *GmailMessage, query string) bool {
	query = strings.ToLower(query)

	// Simple query parsing (Gmail supports complex queries)
	// Format: "from:email to:email subject:text after:date before:date"

	parts := strings.Fields(query)
	for _, part := range parts {
		if strings.HasPrefix(part, "from:") {
			from := strings.TrimPrefix(part, "from:")
			if !e.headerContains(msg, "From", from) {
				return false
			}
		} else if strings.HasPrefix(part, "to:") {
			to := strings.TrimPrefix(part, "to:")
			if !e.headerContains(msg, "To", to) {
				return false
			}
		} else if strings.HasPrefix(part, "subject:") {
			subject := strings.TrimPrefix(part, "subject:")
			if !e.headerContains(msg, "Subject", subject) {
				return false
			}
		} else if strings.HasPrefix(part, "after:") {
			// Parse date and compare
			dateStr := strings.TrimPrefix(part, "after:")
			if after, err := parseQueryDate(dateStr); err == nil {
				msgTime := e.getMessageTime(msg)
				if msgTime.Before(after) {
					return false
				}
			}
		} else if strings.HasPrefix(part, "before:") {
			dateStr := strings.TrimPrefix(part, "before:")
			if before, err := parseQueryDate(dateStr); err == nil {
				msgTime := e.getMessageTime(msg)
				if msgTime.After(before) {
					return false
				}
			}
		} else {
			// General text search in subject and snippet
			found := false
			if e.headerContains(msg, "Subject", part) || strings.Contains(strings.ToLower(msg.Snippet), part) {
				found = true
			}
			if !found {
				return false
			}
		}
	}

	return true
}

func (e *GmailEmulator) headerContains(msg *GmailMessage, headerName, value string) bool {
	if msg.Payload == nil {
		return false
	}

	for _, header := range msg.Payload.Headers {
		if header.Name == headerName {
			return strings.Contains(strings.ToLower(header.Value), strings.ToLower(value))
		}
	}

	return false
}

func (e *GmailEmulator) getMessageTime(msg *GmailMessage) time.Time {
	if ts, err := strconv.ParseInt(msg.InternalDate, 10, 64); err == nil {
		return time.Unix(ts/1000, 0)
	}
	return time.Now()
}

func parseQueryDate(dateStr string) (time.Time, error) {
	// Gmail supports various formats: 2024/1/1, 2024-01-01, etc.
	formats := []string{
		"2006/1/2",
		"2006-01-02",
		"01/02/2006",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	// Also support relative dates like "1d", "1w", "1m"
	if strings.HasSuffix(dateStr, "d") {
		if days, err := strconv.Atoi(strings.TrimSuffix(dateStr, "d")); err == nil {
			return time.Now().AddDate(0, 0, -days), nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
}

func (e *GmailEmulator) countThreads() int {
	threads := make(map[string]bool)
	for _, msg := range e.messages {
		threads[msg.ThreadId] = true
	}
	return len(threads)
}

func (e *GmailEmulator) logRequest(r *http.Request) {
	e.requestLog = append(e.requestLog, RequestLog{
		Method:    r.Method,
		Path:      r.URL.Path,
		Query:     r.URL.RawQuery,
		Timestamp: time.Now(),
	})
}

// OAuth endpoints (mock implementation)

func (e *GmailEmulator) handleOAuth(w http.ResponseWriter, r *http.Request) {
	// Mock OAuth response
	response := map[string]interface{}{
		"access_token":  "mock-access-token",
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": "mock-refresh-token",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Health check
func (e *GmailEmulator) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":   "healthy",
		"messages": len(e.messages),
		"threads":  e.countThreads(),
		"users":    len(e.userList),
		"uptime":   time.Since(startTime).String(),
		"requests": len(e.requestLog),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// Debug endpoints
func (e *GmailEmulator) handleDebugStats(w http.ResponseWriter, r *http.Request) {
	labelStats := make(map[string]int)
	for _, msg := range e.messages {
		for _, label := range msg.LabelIds {
			labelStats[label]++
		}
	}

	// Get top users
	topUsers := e.userList
	if len(topUsers) > 10 {
		topUsers = topUsers[:10]
	}

	stats := map[string]interface{}{
		"totalMessages":     len(e.messages),
		"totalThreads":      e.countThreads(),
		"totalUsers":        len(e.userList),
		"labelDistribution": labelStats,
		"topUsers":          topUsers,
		"recentRequests":    e.requestLog[max(0, len(e.requestLog)-10):],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// New endpoint to list available endpoints
func (e *GmailEmulator) handleListEndpoints(w http.ResponseWriter, r *http.Request) {
	endpoints := []map[string]string{
		{
			"method":      "GET",
			"path":        "/gmail/v1/users/{userId}/profile",
			"description": "Get user profile",
		},
		{
			"method":      "GET",
			"path":        "/gmail/v1/users/{userId}/labels",
			"description": "List labels",
		},
		{
			"method":      "GET",
			"path":        "/gmail/v1/users/{userId}/messages",
			"description": "List messages (supports q, pageToken, maxResults, labelIds parameters)",
		},
		{
			"method":      "GET",
			"path":        "/gmail/v1/users/{userId}/messages/{messageId}",
			"description": "Get a specific message",
		},
		{
			"method":      "POST",
			"path":        "/gmail/v1/users/{userId}/messages/batchGet",
			"description": "Batch get multiple messages",
		},
		{
			"method":      "POST",
			"path":        "/oauth2/v4/token",
			"description": "Mock OAuth token endpoint",
		},
		{
			"method":      "GET",
			"path":        "/health",
			"description": "Health check endpoint",
		},
		{
			"method":      "GET",
			"path":        "/debug/stats",
			"description": "Debug statistics",
		},
		{
			"method":      "GET",
			"path":        "/debug/users",
			"description": "List all email addresses in dataset (supports type and limit parameters)",
		},
		{
			"method":      "GET",
			"path":        "/",
			"description": "This endpoint - lists all available endpoints",
		},
	}

	response := map[string]interface{}{
		"endpoints":   endpoints,
		"version":     "2.3",
		"description": "Gmail API Emulator serving Enron email data",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var startTime = time.Now()

func main() {
	var (
		dataPath  = flag.String("data", "./test-data", "Path to test data directory")
		port      = flag.Int("port", 8080, "Port to listen on")
		userEmail = flag.String("email", "test@example.com", "Test user email address")
	)

	flag.Parse()

	// Create emulator
	emulator, err := NewGmailEmulator(*dataPath, *userEmail)
	if err != nil {
		log.Fatalf("Failed to create emulator: %v", err)
	}

	// Set up routes
	r := mux.NewRouter()

	// Gmail API v1 endpoints
	r.HandleFunc("/gmail/v1/users/{userId}/profile", emulator.handleProfile).Methods("GET")
	r.HandleFunc("/gmail/v1/users/{userId}/labels", emulator.handleLabels).Methods("GET")
	r.HandleFunc("/gmail/v1/users/{userId}/messages", emulator.handleListMessages).Methods("GET")
	r.HandleFunc("/gmail/v1/users/{userId}/messages/{messageId}", emulator.handleGetMessage).Methods("GET")
	r.HandleFunc("/gmail/v1/users/{userId}/messages/batchGet", emulator.handleBatchGet).Methods("POST")

	// OAuth mock endpoints
	r.HandleFunc("/oauth2/v4/token", emulator.handleOAuth).Methods("POST")

	// Health and debug endpoints
	r.HandleFunc("/health", emulator.handleHealth).Methods("GET")
	r.HandleFunc("/debug/stats", emulator.handleDebugStats).Methods("GET")
	r.HandleFunc("/debug/users", emulator.handleListUsers).Methods("GET") // New endpoint

	// Root endpoint - list all endpoints
	r.HandleFunc("/", emulator.handleListEndpoints).Methods("GET")

	// Enable CORS for testing
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	})

	handler := c.Handler(r)

	log.Printf("Gmail API Emulator starting on port %d", *port)
	log.Printf("Serving data from: %s", *dataPath)
	log.Printf("Test user email: %s", *userEmail)
	log.Printf("Health check: http://localhost:%d/health", *port)
	log.Printf("Debug stats: http://localhost:%d/debug/stats", *port)
	log.Printf("User list: http://localhost:%d/debug/users", *port)
	log.Printf("All endpoints: http://localhost:%d/", *port)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
