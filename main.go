package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	apiKey     = "sk-66bb9b2b38214fa29fd402c4b4dd5c41"
	model      = "deepseek-chat"
	apiBaseURL = "https://api.deepseek.com/v1/chat/completions"
	dsn        = "root:00000000@tcp(127.0.0.1:3306)/deepseek_chat_b?charset=utf8mb4&parseTime=True&loc=Local"
)

var db *gorm.DB

type Persona struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"type:varchar(64)" json:"name"`
	Avatar      string    `gorm:"type:varchar(256)" json:"avatar"`
	Identity    string    `gorm:"type:varchar(128)" json:"identity"`
	Appearance  string    `gorm:"type:text" json:"appearance"`
	Personality string    `gorm:"type:text" json:"personality"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Session struct {
	ID          string    `gorm:"primaryKey;type:varchar(64)" json:"id"`
	Name        string    `gorm:"type:varchar(64)" json:"name"`
	Model       string    `gorm:"type:varchar(64)" json:"model"`
	Personality string    `gorm:"type:text" json:"personality"`
	AIName      string    `gorm:"type:varchar(64)" json:"ai_name"`
	AIAvatar    string    `gorm:"type:varchar(256)" json:"ai_avatar"`
	Terminated  bool      `gorm:"type:tinyint(1)" json:"terminated"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	PersonaID   *uint     `gorm:"type:int unsigned" json:"persona_id"`
	Messages    []Message `gorm:"foreignKey:SessionID" json:"messages"`
}

type Message struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SessionID string    `gorm:"type:varchar(64);index" json:"session_id"`
	Role      string    `gorm:"type:varchar(16)" json:"role"`
	Content   string    `gorm:"type:text" json:"content"`
	Meta      string    `gorm:"type:varchar(128)" json:"meta"`
	CreatedAt time.Time `json:"created_at"`
}

type ModelSetupRequest struct {
	ModelName   string `json:"modelName"`
	Personality string `json:"personality"`
	AIName      string `json:"aiName"`
	AIAvatar    string `json:"aiAvatar"`
	PersonaID   *uint  `json:"personaId"`
}

type ChatRequest struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
}

type RenameSessionRequest struct {
	SessionID string `json:"sessionId"`
	NewName   string `json:"newName"`
}

type DeleteSessionRequest struct {
	SessionID string `json:"sessionId"`
}

type TerminateSessionRequest struct {
	SessionID string `json:"sessionId"`
}

type UploadAvatarResponse struct {
	Url string `json:"url"`
}

func main() {
	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("数据库连接失败: ", err)
	}
	if err := db.AutoMigrate(&Session{}, &Message{}, &Persona{}); err != nil {
		log.Fatal("数据库自动迁移失败: ", err)
	}

	r := mux.NewRouter()
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))
	r.HandleFunc("/", serveIndex)
	r.HandleFunc("/api/setup", handleSetup).Methods("POST")
	r.HandleFunc("/api/chat", handleChat).Methods("POST")
	r.HandleFunc("/api/sessions", getSessions).Methods("GET")
	r.HandleFunc("/api/messages", getMessages).Methods("GET")
	r.HandleFunc("/api/session/delete", deleteSession).Methods("POST")
	r.HandleFunc("/api/session/rename", renameSession).Methods("POST")
	r.HandleFunc("/api/upload_avatar", uploadAvatar).Methods("POST")
	r.HandleFunc("/api/session/terminate", terminateSession).Methods("POST")
	// 人格相关
	r.HandleFunc("/api/personas", getPersonas).Methods("GET")
	r.HandleFunc("/api/persona", createOrUpdatePersona).Methods("POST")
	r.HandleFunc("/api/persona/{id}", getPersonaByID).Methods("GET")
	r.HandleFunc("/api/persona/{id}", deletePersona).Methods("DELETE")
	r.HandleFunc("/api/session/use_persona", usePersonaForSession).Methods("POST")

	fmt.Println("服务器启动在 http://localhost:8888")
	log.Fatal(http.ListenAndServe(":8888", r))
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/index.html")
}

func buildSystemMessageFromPersona(p Persona) string {
	systemMsg := fmt.Sprintf("你是一个名为%s的AI助手。", p.Name)
	if p.Identity != "" {
		systemMsg += fmt.Sprintf(" 你的身份是：%s。", p.Identity)
	}
	if p.Appearance != "" {
		systemMsg += fmt.Sprintf(" 你的外貌特征：%s。", p.Appearance)
	}
	if p.Personality != "" {
		systemMsg += fmt.Sprintf(" 你的人格特点：%s。", p.Personality)
	}
	systemMsg += " 请简洁、准确地回答用户的问题。"
	return systemMsg
}

func handleSetup(w http.ResponseWriter, r *http.Request) {
	var req ModelSetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	var persona *Persona
	if req.PersonaID != nil {
		var p Persona
		if err := db.First(&p, *req.PersonaID).Error; err == nil {
			persona = &p
		}
	}

	sessionID := generateSessionID()
	session := Session{
		ID:         sessionID,
		Name:       "新对话",
		Model:      req.ModelName,
		Terminated: false,
	}
	if persona != nil {
		session.Personality = persona.Personality
		session.AIName = persona.Name
		session.AIAvatar = persona.Avatar
		session.PersonaID = &persona.ID
	} else {
		session.Personality = req.Personality
		session.AIName = req.AIName
		session.AIAvatar = req.AIAvatar
	}
	if session.AIName == "" {
		session.AIName = "AI助手"
	}
	if session.AIAvatar == "" {
		session.AIAvatar = "/static/ai_avatar.png"
	}
	if err := db.Create(&session).Error; err != nil {
		http.Error(w, "会话创建失败", http.StatusInternalServerError)
		return
	}
	var sysMsg Message
	if persona != nil {
		sysMsg = Message{
			SessionID: sessionID,
			Role:      "system",
			Content:   buildSystemMessageFromPersona(*persona),
		}
	} else {
		sysMsg = Message{
			SessionID: sessionID,
			Role:      "system",
			Content:   buildSystemMessage(req.ModelName, req.Personality),
		}
	}
	db.Create(&sysMsg)
	response := map[string]string{
		"sessionId": sessionID,
		"message":   "模型设置成功",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// 新增：用AI识别退出意图
func checkExitIntent(userInput string, personality string) bool {
	prompt := fmt.Sprintf(`你是一个AI助手，你的人格特点为：%s。
用户刚才说的话是：“%s”。
请判断用户是否有“结束/退出/终止/再见/不再聊”等终止本次对话的意图。
如果有请只回答"YES"，否则请只回答"NO"。不要输出其他内容。`, personality, userInput)
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", apiBaseURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return false
	}
	if len(apiResp.Choices) > 0 {
		ans := strings.TrimSpace(strings.ToUpper(apiResp.Choices[0].Message.Content))
		return ans == "YES"
	}
	return false
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	var session Session
	if err := db.Where("id = ?", req.SessionID).First(&session).Error; err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	if session.Terminated {
		http.Error(w, "对话已终止", http.StatusForbidden)
		return
	}

	// 1. 判断是否有退出意图
	var personality string
	if session.PersonaID != nil && *session.PersonaID > 0 {
		var persona Persona
		if err := db.First(&persona, *session.PersonaID).Error; err == nil {
			personality = persona.Personality
		}
	}
	if personality == "" {
		personality = session.Personality
	}

	if checkExitIntent(req.Message, personality) {
		// 自动终止流程
		var msgs []Message
		if err := db.Where("session_id = ?", req.SessionID).Order("created_at asc").Find(&msgs).Error; err != nil {
			http.Error(w, "获取消息失败", http.StatusInternalServerError)
			return
		}
		var allContents []string
		for _, m := range msgs {
			allContents = append(allContents, fmt.Sprintf("[%s]: %s", m.Role, m.Content))
		}
		allText := strings.Join(allContents, "\n")

		var summary, newTitle string
		summary, newTitle = summarizeAndTitleByAI(personality, allText)
		if newTitle == "" {
			newTitle = "对话总结"
		}
		db.Model(&Session{}).Where("id = ?", req.SessionID).Updates(map[string]interface{}{
			"terminated": true,
			"name":       newTitle,
		})
		endMsg := Message{
			SessionID: req.SessionID,
			Role:      "system",
			Content:   "本次会话已结束，感谢您的使用",
		}
		db.Create(&endMsg)
		summaryMsg := Message{
			SessionID: req.SessionID,
			Role:      "assistant",
			Content:   summary,
			Meta:      "对话总结",
		}
		db.Create(&summaryMsg)
		// 返回与terminate一致
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"terminated": true,
			"endMessage": "本次会话已结束，感谢您的使用",
			"summary":    summary,
			"newTitle":   newTitle,
		})
		return
	}

	// --- 正常对话流程 ---
	var msgs []Message
	if err := db.Where("session_id = ?", req.SessionID).Order("created_at asc").Find(&msgs).Error; err != nil {
		http.Error(w, "获取历史消息失败", http.StatusInternalServerError)
		return
	}

	// === 构造system prompt ===
	var systemPrompt string
	if session.PersonaID != nil && *session.PersonaID > 0 {
		var persona Persona
		if err := db.First(&persona, *session.PersonaID).Error; err == nil {
			systemPrompt = buildSystemMessageFromPersona(persona)
		}
	}
	if systemPrompt == "" {
		systemPrompt = buildSystemMessage(session.Model, session.Personality)
	}

	// 替换system消息
	chatMsgs := []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{}
	systemAdded := false
	for _, m := range msgs {
		if m.Role == "system" && !systemAdded {
			chatMsgs = append(chatMsgs, struct {
				Role    string "json:\"role\""
				Content string "json:\"content\""
			}{Role: "system", Content: systemPrompt})
			systemAdded = true
		} else if m.Role != "system" {
			chatMsgs = append(chatMsgs, struct {
				Role    string "json:\"role\""
				Content string "json:\"content\""
			}{Role: m.Role, Content: m.Content})
		}
	}
	if !systemAdded {
		chatMsgs = append([]struct {
			Role    string "json:\"role\""
			Content string "json:\"content\""
		}{{Role: "system", Content: systemPrompt}}, chatMsgs...)
	}
	chatMsgs = append(chatMsgs, struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "user", Content: req.Message})

	var userMsgCount int64
	db.Model(&Message{}).Where("session_id = ? AND role = ?", req.SessionID, "user").Count(&userMsgCount)

	userMsg := Message{
		SessionID: req.SessionID,
		Role:      "user",
		Content:   req.Message,
	}
	db.Create(&userMsg)

	if userMsgCount == 0 {
		go func(sessID, personality, message string) {
			title := generateTitleByAI(personality, message)
			if title == "" {
				title = "主题对话"
			}
			db.Model(&Session{}).Where("id = ?", sessID).Update("name", title)
		}(req.SessionID, session.Personality, req.Message)
	}

	startTime := time.Now()
	response, err := callDeepseekAPI(chatMsgs)
	elapsedTime := time.Since(startTime)
	if err != nil {
		http.Error(w, fmt.Sprintf("API调用失败: %v", err), http.StatusInternalServerError)
		return
	}
	reply := response.Choices[0].Message.Content
	aiMsg := Message{
		SessionID: req.SessionID,
		Role:      "assistant",
		Content:   reply,
		Meta:      fmt.Sprintf("响应时间: %s", formatDuration(elapsedTime)),
	}
	db.Create(&aiMsg)

	chatResponse := map[string]interface{}{
		"message":     reply,
		"elapsedTime": formatDuration(elapsedTime),
		"usage":       response.Usage,
		"aiName":      session.AIName,
		"aiAvatar":    session.AIAvatar,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatResponse)
}

// 人格详情
func getPersonaByID(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var p Persona
	if err := db.First(&p, id).Error; err != nil {
		http.Error(w, "未找到该人格", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func terminateSession(w http.ResponseWriter, r *http.Request) {
	var req TerminateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionID == "" {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	var session Session
	if err := db.Where("id = ?", req.SessionID).First(&session).Error; err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	if session.Terminated {
		http.Error(w, "对话已终止", http.StatusBadRequest)
		return
	}
	var msgs []Message
	if err := db.Where("session_id = ?", req.SessionID).Order("created_at asc").Find(&msgs).Error; err != nil {
		http.Error(w, "获取消息失败", http.StatusInternalServerError)
		return
	}
	var allContents []string
	for _, m := range msgs {
		allContents = append(allContents, fmt.Sprintf("[%s]: %s", m.Role, m.Content))
	}
	allText := strings.Join(allContents, "\n")

	summary, newTitle := summarizeAndTitleByAI(session.Personality, allText)
	if newTitle == "" {
		newTitle = "对话总结"
	}
	db.Model(&Session{}).Where("id = ?", req.SessionID).Updates(map[string]interface{}{
		"terminated": true,
		"name":       newTitle,
	})

	endMsg := Message{
		SessionID: req.SessionID,
		Role:      "system",
		Content:   "本次会话已结束，感谢您的使用",
	}
	db.Create(&endMsg)
	summaryMsg := Message{
		SessionID: req.SessionID,
		Role:      "assistant",
		Content:   summary,
		Meta:      "对话总结",
	}
	db.Create(&summaryMsg)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "success", "newTitle": newTitle})
}

func summarizeAndTitleByAI(personality, allText string) (string, string) {
	prompt := fmt.Sprintf("你是一个AI助手，人格特点：%s。请总结以下对话内容，并用一句话（不超过20字）生成一个合适的标题。\n\n对话内容：\n%s\n\n请先输出对话总结，再输出标题（格式：总结\\n标题：xxxx）。", personality, allText)
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", apiBaseURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "对话总结失败", ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "对话总结失败", ""
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "对话总结失败", ""
	}
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "对话总结失败", ""
	}
	if len(apiResp.Choices) > 0 {
		out := strings.TrimSpace(apiResp.Choices[0].Message.Content)
		summary := out
		newTitle := ""
		if idx := strings.LastIndex(out, "标题："); idx != -1 {
			summary = strings.TrimSpace(out[:idx])
			newTitle = strings.TrimSpace(out[idx+len("标题："):])
		}
		return summary, newTitle
	}
	return "对话总结失败", ""
}

func uploadAvatar(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	file, handler, err := r.FormFile("avatar")
	if err != nil {
		http.Error(w, "文件上传失败", http.StatusBadRequest)
		return
	}
	defer file.Close()
	ext := filepath.Ext(handler.Filename)
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		http.Error(w, "仅支持PNG/JPG/JPEG", http.StatusBadRequest)
		return
	}
	saveDir := "static/avatars"
	os.MkdirAll(saveDir, 0755)
	fileName := fmt.Sprintf("avatar_%d%s", time.Now().UnixNano(), ext)
	savePath := filepath.Join(saveDir, fileName)
	out, err := os.Create(savePath)
	if err != nil {
		http.Error(w, "文件保存失败", http.StatusInternalServerError)
		return
	}
	defer out.Close()
	io.Copy(out, file)
	url := "/static/avatars/" + fileName
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(UploadAvatarResponse{Url: url})
}

func getSessions(w http.ResponseWriter, r *http.Request) {
	var sessions []Session
	if err := db.Order("created_at desc").Find(&sessions).Error; err != nil {
		http.Error(w, "获取会话失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

func getMessages(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "缺少sessionId参数", http.StatusBadRequest)
		return
	}
	var msgs []Message
	if err := db.Where("session_id = ?", sessionID).Order("created_at asc").Find(&msgs).Error; err != nil {
		http.Error(w, "获取消息失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}

func deleteSession(w http.ResponseWriter, r *http.Request) {
	var req DeleteSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionID == "" {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := db.Where("session_id = ?", req.SessionID).Delete(&Message{}).Error; err != nil {
		http.Error(w, "消息删除失败", http.StatusInternalServerError)
		return
	}
	if err := db.Where("id = ?", req.SessionID).Delete(&Session{}).Error; err != nil {
		http.Error(w, "会话删除失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "success"})
}

func renameSession(w http.ResponseWriter, r *http.Request) {
	var req RenameSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SessionID == "" || req.NewName == "" {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if err := db.Model(&Session{}).Where("id = ?", req.SessionID).Update("name", req.NewName).Error; err != nil {
		http.Error(w, "重命名失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "success"})
}

func generateTitleByAI(personality, firstMsg string) string {
	prompt := "你是一个AI助手，用户的人格特点是：" + personality + "。用户的对话主题如下：" + firstMsg + "。请用一句话（不超过20字）为本次对话生成一个简洁、准确的标题。直接返回标题，不要多余的话。"
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	jsonBody, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", apiBaseURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return ""
	}
	if len(apiResp.Choices) > 0 {
		return strings.TrimSpace(apiResp.Choices[0].Message.Content)
	}
	return ""
}

func generateSessionID() string {
	return fmt.Sprintf("session_%d", time.Now().UnixNano())
}
func buildSystemMessage(modelName, personality string) string {
	systemMsg := fmt.Sprintf("你是一个名为%s的AI助手。", modelName)
	if personality != "" {
		systemMsg += fmt.Sprintf(" 你的人格特点是: %s", personality)
	}
	systemMsg += " 请简洁、准确地回答用户的问题。"
	return systemMsg
}
func callDeepseekAPI(messages []struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}) (*struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}, error) {
	requestBody := struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Model:    model,
		Messages: messages,
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", apiBaseURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}
	var response struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int    `json:"created"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v, 响应内容: %s", err, string(body))
	}
	return &response, nil
}
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func getPersonas(w http.ResponseWriter, r *http.Request) {
	var personas []Persona
	if err := db.Order("created_at desc").Find(&personas).Error; err != nil {
		http.Error(w, "获取人格失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(personas)
}

func createOrUpdatePersona(w http.ResponseWriter, r *http.Request) {
	var data Persona
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	if data.Name == "" {
		http.Error(w, "名称不能为空", http.StatusBadRequest)
		return
	}
	now := time.Now()
	if data.ID > 0 {
		data.UpdatedAt = now
		if err := db.Model(&Persona{}).Where("id=?", data.ID).Updates(data).Error; err != nil {
			http.Error(w, "更新失败", http.StatusInternalServerError)
			return
		}
	} else {
		data.CreatedAt = now
		data.UpdatedAt = now
		if err := db.Create(&data).Error; err != nil {
			http.Error(w, "创建失败", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"result": "success", "persona": data})
}

func deletePersona(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := db.Delete(&Persona{}, id).Error; err != nil {
		http.Error(w, "删除失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "success"})
}

func usePersonaForSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"sessionId"`
		PersonaID uint   `json:"personaId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "参数错误", http.StatusBadRequest)
		return
	}
	var persona Persona
	if err := db.First(&persona, req.PersonaID).Error; err != nil {
		http.Error(w, "人格不存在", http.StatusBadRequest)
		return
	}
	db.Model(&Session{}).Where("id=?", req.SessionID).Updates(map[string]interface{}{
		"personality": persona.Personality,
		"ai_name":     persona.Name,
		"ai_avatar":   persona.Avatar,
		"persona_id":  persona.ID,
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": "success"})
}
