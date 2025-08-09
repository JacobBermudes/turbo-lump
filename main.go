package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Структура для кэша данных
type Cache struct {
	data []byte
	mu   sync.RWMutex
}

type updResp struct {
	UpdTm string `json:"updTm"`
}

// Глобальный кэш
var cache = &Cache{}

// URL 1С
const oneCUrl = "https://1c.fariante.ru/acc/hs/catalog/all"

func fetchDataFrom1C() (string, error) {

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", oneCUrl, nil)
	if err != nil {
		return "Ошибка создания запроса к 1С: ", err
	}

	pass := os.Getenv("PASS_1C")
	if pass == "" {
		return "Переменная окружения PASS_1C не установлена", err
	}

	req.SetBasicAuth("Интеграция", pass)

	resp, err := client.Do(req)
	if err != nil {
		return "Ошибка получения данных из 1С", err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "Ошибка", fmt.Errorf("статус %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "Неразборные данные в теле", err
	}

	cache.mu.Lock()
	cache.data = data
	cache.mu.Unlock()

	return time.Now().Format(time.DateTime), nil
}

// Возврат кешированных данных
func dataGetHandler(w http.ResponseWriter, r *http.Request) {

	cache.mu.RLock()
	data := cache.data
	cache.mu.RUnlock()

	if data == nil {
		http.Error(w, `{"error": "Данные недоступны, попробуйте позже"}`, http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*") //исправить для прода
	w.Write(data)
}

func dataUpdateHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodPost {
		http.Error(w, `{"error": "Метод не поддерживается"}`, http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*") //исправить для прода

	updDate, err := fetchDataFrom1C()

	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	timing := updResp{
		UpdTm: updDate,
	}

	json.NewEncoder(w).Encode(timing)
}

func main() {

	fetchDataFrom1C()

	http.HandleFunc("/1cgw/data", dataGetHandler)
	http.HandleFunc("/1cgw/update", dataUpdateHandler)

	port := ":3210"
	log.Printf("Сервер запущен на порту %s", port)
	//if err := http.ListenAndServeTLS(port, "/etc/letsencrypt/live/fariante.ru/fullchain.pem", "/etc/letsencrypt/live/fariante.ru/privkey.pem", nil); err != nil {
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}
