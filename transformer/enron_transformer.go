// enron_transformer.go
//
// Transforms Enron email dataset into Gmail API format for testing
// Version: 2.0
// Last Updated: 2025-07-13
//
// Carson Sweet assisted by Claude AI
// https://www.carsonsweet.com

package main

import (
	"bufio"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// EnronEmail represents an email from the Enron dataset
type EnronEmail struct {
	MessageID string
	Date      time.Time
	From      string
	To        []string
	CC        []string
	BCC       []string
	Subject   string
	Body      string

	// Enron-specific metadata
	XFrom     string
	XTo       string
	XCC       string
	XBCC      string
	XFolder   string
	XOrigin   string
	XFileName string

	// Derived fields
	FilePath   string
	UserFolder string
}

// GmailMessage represents a Gmail API message
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

type TestPersona struct {
	Name         string
	Email        string
	Role         string
	Company      string
	FirstContact time.Time
}

type TransformStats struct {
	TotalProcessed   int
	TotalTransformed int
	Errors           []string
	PersonaMap       map[string]TestPersona
	ThreadCount      int
}

type contact struct {
	email string
	count int
}

// GmailTransformer handles the transformation from Enron to Gmail format
type GmailTransformer struct {
	baseDate      time.Time
	timeShift     time.Duration
	threadCache   map[string]string
	personaMap    map[string]TestPersona
	messageIDMap  map[string]string
	userEmail     string
	enronUserName string
	stats         TransformStats
}

func NewGmailTransformer(enronUserName, testUserEmail string) *GmailTransformer {
	baseDate := time.Now().AddDate(-3, 0, 0)
	enronStart := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	return &GmailTransformer{
		baseDate:      baseDate,
		timeShift:     baseDate.Sub(enronStart),
		threadCache:   make(map[string]string),
		personaMap:    make(map[string]TestPersona),
		messageIDMap:  make(map[string]string),
		userEmail:     testUserEmail,
		enronUserName: enronUserName,
		stats: TransformStats{
			PersonaMap: make(map[string]TestPersona),
			Errors:     []string{},
		},
	}
}

// LoadEnronEmails loads emails from the Enron dataset
func LoadEnronEmails(rootPath, username string, limit int) ([]*EnronEmail, error) {
	emails := []*EnronEmail{}
	userPath := filepath.Join(rootPath, username)

	// Priority folders for good test data
	priorityFolders := []string{
		"sent_items",
		"inbox",
		"discussion_threads",
		"personal",
	}

	emailsSeen := make(map[string]bool)

	for _, folder := range priorityFolders {
		folderPath := filepath.Join(userPath, folder)

		files, err := ioutil.ReadDir(folderPath)
		if err != nil {
			continue
		}

		for _, file := range files {
			if len(emails) >= limit {
				return emails, nil
			}

			if file.IsDir() {
				continue
			}

			email, err := parseEnronFile(filepath.Join(folderPath, file.Name()))
			if err != nil {
				continue
			}

			if !emailsSeen[email.MessageID] {
				emailsSeen[email.MessageID] = true
				email.UserFolder = folder
				emails = append(emails, email)
			}
		}
	}

	// If we need more, grab from all_documents
	if len(emails) < limit {
		allDocsPath := filepath.Join(userPath, "all_documents")
		files, _ := ioutil.ReadDir(allDocsPath)

		for _, file := range files {
			if len(emails) >= limit {
				break
			}

			if _, err := strconv.Atoi(strings.TrimSuffix(file.Name(), ".")); err == nil {
				email, err := parseEnronFile(filepath.Join(allDocsPath, file.Name()))
				if err == nil && !emailsSeen[email.MessageID] {
					emailsSeen[email.MessageID] = true
					email.UserFolder = "all_documents"
					emails = append(emails, email)
				}
			}
		}
	}

	return emails, nil
}

func parseEnronFile(path string) (*EnronEmail, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	email := &EnronEmail{FilePath: path}
	scanner := bufio.NewScanner(file)

	inHeaders := true
	var body strings.Builder
	currentHeader := ""
	currentValue := ""

	for scanner.Scan() {
		line := scanner.Text()

		if inHeaders {
			if line == "" {
				if currentHeader != "" {
					processHeader(email, currentHeader, currentValue)
				}
				inHeaders = false
				continue
			}

			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				currentValue += " " + strings.TrimSpace(line)
			} else {
				if currentHeader != "" {
					processHeader(email, currentHeader, currentValue)
				}

				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					currentHeader = strings.TrimSpace(parts[0])
					currentValue = strings.TrimSpace(parts[1])
				}
			}
		} else {
			body.WriteString(line + "\n")
		}
	}

	email.Body = body.String()
	return email, nil
}

func processHeader(email *EnronEmail, header, value string) {
	switch header {
	case "Message-ID":
		email.MessageID = value
	case "Date":
		email.Date = parseEnronDate(value)
	case "From":
		email.From = value
	case "To":
		email.To = parseRecipientList(value)
	case "Cc":
		email.CC = parseRecipientList(value)
	case "Bcc":
		email.BCC = parseRecipientList(value)
	case "Subject":
		email.Subject = value
	case "X-From":
		email.XFrom = cleanEnronAddress(value)
	case "X-To":
		email.XTo = value
	case "X-cc":
		email.XCC = value
	case "X-bcc":
		email.XBCC = value
	case "X-Folder":
		email.XFolder = value
	case "X-Origin":
		email.XOrigin = value
	case "X-FileName":
		email.XFileName = value
	}
}

func parseEnronDate(dateStr string) time.Time {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 2 Jan 2006 15:04:05 -0700 (MST)",
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"2 Jan 2006 15:04:05 -0700",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	return time.Now()
}

func cleanEnronAddress(addr string) string {
	if idx := strings.Index(addr, "</O="); idx > 0 {
		return strings.TrimSpace(addr[:idx])
	}
	return addr
}

func parseRecipientList(value string) []string {
	recipients := []string{}
	parts := strings.Split(value, ",")
	for _, part := range parts {
		cleaned := strings.TrimSpace(part)
		if cleaned != "" {
			recipients = append(recipients, cleaned)
		}
	}
	return recipients
}

// TransformDataset transforms Enron emails to Gmail format
func (t *GmailTransformer) TransformDataset(emails []*EnronEmail) ([]*GmailMessage, error) {
	sort.Slice(emails, func(i, j int) bool {
		return emails[i].Date.Before(emails[j].Date)
	})

	t.buildPersonaMap(emails)

	gmailMessages := make([]*GmailMessage, 0, len(emails))

	for _, enron := range emails {
		t.stats.TotalProcessed++

		gmail, err := t.transformEmail(enron)
		if err != nil {
			t.stats.Errors = append(t.stats.Errors, fmt.Sprintf("Error transforming %s: %v", enron.MessageID, err))
			continue
		}

		gmailMessages = append(gmailMessages, gmail)
		t.stats.TotalTransformed++
	}

	t.stats.ThreadCount = len(t.threadCache)
	t.stats.PersonaMap = t.personaMap

	return gmailMessages, nil
}

func (t *GmailTransformer) buildPersonaMap(emails []*EnronEmail) {
	contactFreq := make(map[string]int)

	for _, email := range emails {
		from := t.extractEmail(email.From)
		if from != "" && !strings.Contains(from, t.enronUserName) {
			contactFreq[from]++
		}

		for _, to := range email.To {
			addr := t.extractEmail(to)
			if addr != "" && !strings.Contains(addr, t.enronUserName) {
				contactFreq[addr]++
			}
		}
	}

	contacts := []contact{}
	for email, count := range contactFreq {
		contacts = append(contacts, contact{email, count})
	}
	sort.Slice(contacts, func(i, j int) bool {
		return contacts[i].count > contacts[j].count
	})

	t.assignPersonas(contacts)
}

func (t *GmailTransformer) assignPersonas(contacts []contact) {
	personas := []TestPersona{
		{Name: "Sarah Chen", Email: "sarah.chen@gmail.com", Role: "sister", Company: ""},
		{Name: "David Kumar", Email: "david.kumar@techcorp.com", Role: "manager", Company: "TechCorp"},
		{Name: "Alex Rivera", Email: "alex.r@gmail.com", Role: "best friend", Company: ""},
		{Name: "Lisa Thompson", Email: "lisa.t@techcorp.com", Role: "colleague", Company: "TechCorp"},
		{Name: "Mom", Email: "mom.wilson@yahoo.com", Role: "family", Company: ""},
		{Name: "Jamie Park", Email: "jamiepark92@gmail.com", Role: "friend", Company: ""},
		{Name: "Michael Chen", Email: "m.chen@techcorp.com", Role: "colleague", Company: "TechCorp"},
		{Name: "Emma Davis", Email: "emma.davis@gmail.com", Role: "friend", Company: ""},
		{Name: "Robert Johnson", Email: "rjohnson@partnerco.com", Role: "client", Company: "PartnerCo"},
		{Name: "Jessica Lee", Email: "jlee@techcorp.com", Role: "colleague", Company: "TechCorp"},
	}

	for i, c := range contacts {
		if i >= len(personas) {
			t.personaMap[c.email] = TestPersona{
				Name:  t.generateNameFromEmail(c.email),
				Email: fmt.Sprintf("contact%d@example.com", i),
				Role:  "acquaintance",
			}
		} else {
			t.personaMap[c.email] = personas[i]
		}
	}

	serviceEmails := []string{
		"notifications@github.com",
		"no-reply@linkedin.com",
		"united@united.com",
		"alerts@mint.com",
	}

	for _, email := range serviceEmails {
		t.personaMap[email] = TestPersona{
			Name:  t.extractServiceName(email),
			Email: email,
			Role:  "service",
		}
	}
}

func (t *GmailTransformer) transformEmail(enron *EnronEmail) (*GmailMessage, error) {
	gmailID := t.generateGmailID(enron.MessageID)
	t.messageIDMap[enron.MessageID] = gmailID

	headers := t.transformHeaders(enron)
	threadID := t.getOrCreateThreadID(enron)
	body := t.transformBody(enron.Body)

	payload := &MessagePart{
		PartId:   "",
		MimeType: "text/plain",
		Headers:  headers,
		Body: &MessageBody{
			Size: len(body),
			Data: base64.StdEncoding.EncodeToString([]byte(body)),
		},
	}

	labels := t.inferLabels(enron)
	shiftedDate := enron.Date.Add(t.timeShift)

	gmail := &GmailMessage{
		Id:           gmailID,
		ThreadId:     threadID,
		LabelIds:     labels,
		Snippet:      t.generateSnippet(body),
		HistoryId:    fmt.Sprintf("%d", shiftedDate.Unix()),
		InternalDate: fmt.Sprintf("%d", shiftedDate.UnixMilli()),
		SizeEstimate: len(enron.Body) + 512,
		Payload:      payload,
	}

	return gmail, nil
}

func (t *GmailTransformer) transformHeaders(enron *EnronEmail) []Header {
	headers := []Header{}

	from := t.transformEmailAddress(enron.From)
	headers = append(headers, Header{Name: "From", Value: from})

	if len(enron.To) > 0 {
		to := t.transformEmailList(enron.To)
		headers = append(headers, Header{Name: "To", Value: to})
	}

	if len(enron.CC) > 0 {
		cc := t.transformEmailList(enron.CC)
		headers = append(headers, Header{Name: "Cc", Value: cc})
	}

	headers = append(headers,
		Header{Name: "Subject", Value: enron.Subject},
		Header{Name: "Date", Value: enron.Date.Add(t.timeShift).Format(time.RFC1123Z)},
		Header{Name: "Message-ID", Value: fmt.Sprintf("<%s@mail.gmail.com>", t.generateGmailID(enron.MessageID))},
	)

	return headers
}

func (t *GmailTransformer) transformEmailAddress(enronEmail string) string {
	cleaned := t.extractEmail(enronEmail)

	if strings.Contains(cleaned, t.enronUserName) {
		return fmt.Sprintf("%s <%s>", "You", t.userEmail)
	}

	if persona, ok := t.personaMap[cleaned]; ok {
		return fmt.Sprintf("%s <%s>", persona.Name, persona.Email)
	}

	name := t.generateNameFromEmail(cleaned)
	domain := "example.com"
	if strings.Contains(cleaned, "@") {
		parts := strings.Split(cleaned, "@")
		if parts[1] != "enron.com" {
			domain = parts[1]
		}
	}

	return fmt.Sprintf("%s <%s@%s>", name, strings.ToLower(name), domain)
}

func (t *GmailTransformer) transformEmailList(emails []string) string {
	transformed := []string{}
	for _, email := range emails {
		transformed = append(transformed, t.transformEmailAddress(email))
	}
	return strings.Join(transformed, ", ")
}

func (t *GmailTransformer) getOrCreateThreadID(enron *EnronEmail) string {
	subject := t.cleanSubjectForThreading(enron.Subject)

	participants := []string{enron.From}
	participants = append(participants, enron.To...)
	sort.Strings(participants)

	if len(participants) > 3 {
		participants = participants[:3]
	}

	threadKey := fmt.Sprintf("%s|%s", subject, strings.Join(participants, ","))

	if threadID, exists := t.threadCache[threadKey]; exists {
		return threadID
	}

	threadID := t.generateThreadID(threadKey)
	t.threadCache[threadKey] = threadID

	return threadID
}

func (t *GmailTransformer) cleanSubjectForThreading(subject string) string {
	prefixes := regexp.MustCompile(`^(Re:|RE:|Fwd:|FW:|Fw:)\s*`)
	cleaned := prefixes.ReplaceAllString(subject, "")
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.ToLower(cleaned)
	return cleaned
}

func (t *GmailTransformer) inferLabels(enron *EnronEmail) []string {
	labels := []string{"UNREAD"}

	if strings.Contains(enron.From, t.enronUserName) {
		labels = append(labels, "SENT")
	} else {
		labels = append(labels, "INBOX")
	}

	folder := strings.ToLower(enron.UserFolder)

	switch {
	case strings.Contains(folder, "trash") || strings.Contains(folder, "deleted"):
		labels = append(labels, "TRASH")
	case strings.Contains(folder, "personal"):
		labels = append(labels, "CATEGORY_PERSONAL")
	case strings.Contains(folder, "travel"):
		labels = append(labels, "Label_Travel")
	case strings.Contains(folder, "conferences") || strings.Contains(folder, "meetings"):
		labels = append(labels, "Label_Meetings")
	}

	if t.isImportant(enron) {
		labels = append(labels, "IMPORTANT")
	}

	if t.isPromotional(enron) {
		labels = append(labels, "CATEGORY_PROMOTIONS")
	} else if t.isAutomated(enron) {
		labels = append(labels, "CATEGORY_UPDATES")
	}

	return labels
}

func (t *GmailTransformer) isImportant(enron *EnronEmail) bool {
	subject := strings.ToLower(enron.Subject)
	body := strings.ToLower(enron.Body)

	importantPatterns := []string{
		"urgent", "asap", "important", "critical",
		"action required", "deadline", "immediate",
		"confidential", "board meeting", "executive",
	}

	for _, pattern := range importantPatterns {
		if strings.Contains(subject, pattern) || strings.Contains(body[:min(200, len(body))], pattern) {
			return true
		}
	}

	if strings.Contains(enron.From, "gibner") ||
		strings.Contains(enron.From, "buy") ||
		strings.Contains(enron.From, "lay") {
		return true
	}

	return false
}

func (t *GmailTransformer) isPromotional(enron *EnronEmail) bool {
	indicators := []string{
		"unsubscribe", "click here", "special offer",
		"deal", "discount", "sale", "free shipping",
		"act now", "limited time",
	}

	content := strings.ToLower(enron.Subject + " " + enron.Body)
	for _, indicator := range indicators {
		if strings.Contains(content, indicator) {
			return true
		}
	}

	return false
}

func (t *GmailTransformer) isAutomated(enron *EnronEmail) bool {
	from := strings.ToLower(enron.From)

	automatedPatterns := []string{
		"no-reply", "noreply", "donotreply",
		"notification", "alert", "system",
		"automated", "mailman", "listserv",
	}

	for _, pattern := range automatedPatterns {
		if strings.Contains(from, pattern) {
			return true
		}
	}

	return false
}

func (t *GmailTransformer) generateSnippet(body string) string {
	cleaned := strings.TrimSpace(body)
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")

	lines := strings.Split(cleaned, "\n")
	snippet := ""

	for _, line := range lines {
		if strings.HasPrefix(line, ">") || strings.Contains(line, "-----Original Message-----") {
			break
		}
		snippet += line + " "
		if len(snippet) > 100 {
			break
		}
	}

	if len(snippet) > 100 {
		snippet = snippet[:97] + "..."
	}

	return strings.TrimSpace(snippet)
}

func (t *GmailTransformer) transformBody(body string) string {
	body = t.fixEncoding(body)

	emailRegex := regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)

	body = emailRegex.ReplaceAllStringFunc(body, func(email string) string {
		cleaned := strings.ToLower(email)

		if strings.Contains(cleaned, t.enronUserName) {
			return t.userEmail
		}

		if persona, ok := t.personaMap[cleaned]; ok {
			return persona.Email
		}

		return email
	})

	body = strings.ReplaceAll(body, "Enron", "TechCorp")
	body = strings.ReplaceAll(body, "ENRON", "TECHCORP")

	return body
}

func (t *GmailTransformer) fixEncoding(text string) string {
	replacements := map[string]string{
		"=20":   " ",
		"=09":   "\t",
		"=0A":   "\n",
		"=0D":   "\r",
		"=3D":   "=",
		"&amp;": "&",
		"&lt;":  "<",
		"&gt;":  ">",
	}

	for old, new := range replacements {
		text = strings.ReplaceAll(text, old, new)
	}

	return text
}

func (t *GmailTransformer) extractEmail(address string) string {
	if idx := strings.Index(address, "<"); idx >= 0 {
		if endIdx := strings.Index(address[idx:], ">"); endIdx > 0 {
			return strings.ToLower(address[idx+1 : idx+endIdx])
		}
	}

	if strings.Contains(address, "@") {
		return strings.ToLower(strings.TrimSpace(address))
	}

	return ""
}

func (t *GmailTransformer) generateNameFromEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) == 0 {
		return "Unknown"
	}

	name := parts[0]
	name = strings.ReplaceAll(name, ".", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	return strings.Title(name)
}

func (t *GmailTransformer) extractServiceName(email string) string {
	domain := strings.Split(email, "@")[1]
	name := strings.Split(domain, ".")[0]
	return strings.Title(name)
}

func (t *GmailTransformer) generateGmailID(enronMessageID string) string {
	hash := md5.Sum([]byte(enronMessageID))
	return fmt.Sprintf("%x", hash)[:16]
}

func (t *GmailTransformer) generateThreadID(key string) string {
	hash := md5.Sum([]byte("thread:" + key))
	return fmt.Sprintf("%x", hash)[:16]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Gmail API Response structures
type ListMessagesResponse struct {
	Messages           []MessageRef `json:"messages"`
	NextPageToken      string       `json:"nextPageToken,omitempty"`
	ResultSizeEstimate int          `json:"resultSizeEstimate"`
}

type MessageRef struct {
	Id       string `json:"id"`
	ThreadId string `json:"threadId"`
}

// GenerateTestData creates all necessary test fixtures
func GenerateTestData(messages []*GmailMessage, outputDir string) error {
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// 1. Save raw Gmail messages
	messagesFile := filepath.Join(outputDir, "gmail_messages.json")
	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}
	if err := ioutil.WriteFile(messagesFile, data, 0644); err != nil {
		return fmt.Errorf("write messages: %w", err)
	}

	// 2. Generate message list response
	messageRefs := make([]MessageRef, len(messages))
	for i, msg := range messages {
		messageRefs[i] = MessageRef{
			Id:       msg.Id,
			ThreadId: msg.ThreadId,
		}
	}

	listResponse := ListMessagesResponse{
		Messages:           messageRefs,
		ResultSizeEstimate: len(messages),
	}

	listFile := filepath.Join(outputDir, "list_messages_response.json")
	data, err = json.MarshalIndent(listResponse, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal list response: %w", err)
	}
	if err := ioutil.WriteFile(listFile, data, 0644); err != nil {
		return fmt.Errorf("write list response: %w", err)
	}

	// 3. Generate test metadata
	metadata := map[string]interface{}{
		"totalMessages": len(messages),
		"dateRange": map[string]string{
			"start": messages[0].Payload.Headers[4].Value,
			"end":   messages[len(messages)-1].Payload.Headers[4].Value,
		},
		"labelDistribution": getLabelDistribution(messages),
		"threadCount":       getThreadCount(messages),
	}

	metaFile := filepath.Join(outputDir, "test_metadata.json")
	data, err = json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	if err := ioutil.WriteFile(metaFile, data, 0644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	return nil
}

func getLabelDistribution(messages []*GmailMessage) map[string]int {
	dist := make(map[string]int)
	for _, msg := range messages {
		for _, label := range msg.LabelIds {
			dist[label]++
		}
	}
	return dist
}

func getThreadCount(messages []*GmailMessage) int {
	threads := make(map[string]bool)
	for _, msg := range messages {
		threads[msg.ThreadId] = true
	}
	return len(threads)
}

func main() {
	var (
		enronPath = flag.String("enron-path", "", "Path to Enron maildir dataset")
		user      = flag.String("user", "kaminski-v", "Enron username to process")
		limit     = flag.Int("limit", 5000, "Maximum number of emails to process")
		outputDir = flag.String("output", "./test-data", "Output directory for test data")
		testEmail = flag.String("test-email", "test@example.com", "Test user email address")
	)

	flag.Parse()

	if *enronPath == "" {
		log.Fatal("--enron-path is required")
	}

	log.Printf("Loading emails from %s/%s...\n", *enronPath, *user)

	emails, err := LoadEnronEmails(*enronPath, *user, *limit)
	if err != nil {
		log.Fatalf("Failed to load emails: %v", err)
	}

	log.Printf("Loaded %d emails\n", len(emails))

	transformer := NewGmailTransformer(strings.Split(*user, "-")[0], *testEmail)
	gmailMessages, err := transformer.TransformDataset(emails)
	if err != nil {
		log.Fatalf("Failed to transform emails: %v", err)
	}

	log.Printf("Transformed %d emails\n", len(gmailMessages))
	log.Printf("Created %d threads\n", transformer.stats.ThreadCount)
	log.Printf("Mapped %d personas\n", len(transformer.stats.PersonaMap))

	if len(transformer.stats.Errors) > 0 {
		log.Printf("Errors during transformation: %d\n", len(transformer.stats.Errors))
		for i, err := range transformer.stats.Errors {
			if i < 5 {
				log.Printf("  - %s\n", err)
			}
		}
	}

	// Save test data
	if err := GenerateTestData(gmailMessages, *outputDir); err != nil {
		log.Fatalf("Failed to generate test data: %v", err)
	}

	// Save transformation stats
	statsFile := filepath.Join(*outputDir, "transform_stats.json")
	statsData, _ := json.MarshalIndent(transformer.stats, "", "  ")
	ioutil.WriteFile(statsFile, statsData, 0644)

	log.Printf("Test data generated in %s\n", *outputDir)
	log.Printf("Files created:\n")
	log.Printf("  - gmail_messages.json: Full message data\n")
	log.Printf("  - list_messages_response.json: API list response\n")
	log.Printf("  - test_metadata.json: Dataset statistics\n")
	log.Printf("  - transform_stats.json: Transformation details\n")
}
