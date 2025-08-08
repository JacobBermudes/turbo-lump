package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Структура для кэша данных
type Cache struct {
	data []byte
	mu   sync.RWMutex
}

// Глобальный кэш
var cache = &Cache{}

// URL 1С
const oneCUrl = "https://1c.fariante.ru/acc/hs/catalog/all"

func fetchDataFrom1C() {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", oneCUrl, nil)
	if err != nil {
		log.Printf("Ошибка создания запроса к 1С: %v", err)
		return
	}

	pass := os.Getenv("PASS_1C")
	if pass == "" {
		log.Println("Переменная окружения PASS_1C не установлена")
		return
	}

	req.SetBasicAuth("Интеграция", pass)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Ошибка получения данных из 1С: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("1С вернул статус: %d", resp.StatusCode)
		return
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Ошибка чтения данных из 1С: %v", err)
		return
	}

	cache.mu.Lock()
	cache.data = data
	cache.mu.Unlock()

	log.Printf("Данные из 1С обновлены: %s", time.Now().Format(time.RFC3339))
}

// Возврат кешированных данных
func handleData(w http.ResponseWriter, r *http.Request) {
	cache.mu.RLock()
	data := cache.data
	cache.mu.RUnlock()

	if data == nil {
		http.Error(w, `{"error": "Данные недоступны, попробуйте позже"}`, http.StatusServiceUnavailable)
		return
	}

	log.Printf("Отправка данных из кэша: %s", time.Now().Format(time.RFC3339))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(data)
}

func main() {
	fetchDataFrom1C()

	// Обновление данных из 1С каждый час
	c := cron.New()
	_, err := c.AddFunc("@hourly", fetchDataFrom1C)
	if err != nil {
		log.Fatalf("Ошибка настройки планировщика: %v", err)
	}
	c.Start()

	// Настройка HTTP-сервера
	http.HandleFunc("/1cgw/data", handleData)

	port := ":3210"
	log.Printf("Сервер запущен на порту %s", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Ошибка запуска сервера: %v", err)
	}
}
