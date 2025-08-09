package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-redis/redis"
)

type Product struct {
	Name  string `json:"name"`
	Artc  string `json:"artc"`
	Units string `json:"units"`
}

type updResp struct {
	UpdTm string `json:"updTm"`
}

var rdb *redis.Client

// URL 1С
const oneCUrl = "https://1c.fariante.ru/acc/hs/catalog/all"

func init() {
	redispass := os.Getenv("PASS_REDIS")
	if redispass == "" {
		log.Fatal("Переменная окружения PASS_REDIS не установлена")
	}

	rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: redispass,
		DB:       0,
	})
	fmt.Printf("Подключение к Redis: %s\n", rdb.Options().Addr)
}

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

	bodyRed, err := io.ReadAll(resp.Body)
	if err != nil {
		return "Неразборные данные в теле", err
	}

	var products []Product
	if err := json.Unmarshal(bodyRed, &products); err != nil {
		return "Ошибка разбора тела", err
	}

	for _, product := range products {

		dataJSON, err := json.Marshal(product)
		if err != nil {
			return "Ошибка сериализации данных", err
		}

		key := fmt.Sprintf("%s", product.Name)
		err = rdb.Set(key, dataJSON, 0).Err()
		if err != nil {
			return "Ошибка записи в Redis", err
		}
	}

	return time.Now().Format(time.DateTime), nil
}

func dataGetHandler(w http.ResponseWriter, r *http.Request) {

	keys, err := rdb.Keys("*").Result()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Ошибка получения ключей из Redis: %v"}`, err), http.StatusInternalServerError)
		return
	}

	var products []Product
	for _, key := range keys {
		data, err := rdb.Get(key).Result()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Ошибка чтения ключа %s из Redis: %v"}`, key, err), http.StatusInternalServerError)
			return
		}

		var product Product
		if err := json.Unmarshal([]byte(data), &product); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Ошибка разбора данных для ключа %s: %v"}`, key, err), http.StatusInternalServerError)
			return
		}
		products = append(products, product)
	}

	data, err := json.Marshal(products)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Ошибка сериализации данных: %v"}`, err), http.StatusInternalServerError)
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
