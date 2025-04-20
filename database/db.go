package database

import (
	"database/sql"
	"encoding/json"
	"httpproxy/dto"
	"log"

	_ "modernc.org/sqlite"
)

var db *sql.DB

func InitDB() {
	var err error
	db, err = sql.Open("sqlite", "data.db")
	if err != nil {
		log.Fatal("failed to open database:", err)
	}

	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS requests (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            method TEXT,
            path TEXT,
            get_params TEXT,
            headers TEXT,
            cookie TEXT,
            body TEXT,
            secure INTEGER,
            rerequest TEXT
        );
        CREATE TABLE IF NOT EXISTS responses (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            request_id INTEGER,
            code INTEGER,
            message TEXT,
            headers TEXT,
            body TEXT,
            FOREIGN KEY(request_id) REFERENCES requests(id)
        );
    `)
	if err != nil {
		log.Fatal("failed to create tables:", err)
	} else {
		log.Println("DB started")
	}
}

func SaveRequestResponse(rr dto.RequestAndResponse) error {
	getParamsJSON, _ := json.Marshal(rr.Request.GetParams)
	headersJSON, _ := json.Marshal(rr.Request.Headers)
	cookieJSON, _ := json.Marshal(rr.Request.Cookie)
	respHeadersJSON, _ := json.Marshal(rr.Response.Headers)

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	res, err := tx.Exec(`INSERT INTO requests (method, path, get_params, headers, cookie, body, secure, rerequest) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rr.Request.Method, rr.Request.Path, getParamsJSON, headersJSON, cookieJSON, rr.Request.Body, rr.Request.Secure, rr.Request.FastRerequests)
	if err != nil {
		tx.Rollback()
		return err
	}

	reqID, _ := res.LastInsertId()
	_, err = tx.Exec(`INSERT INTO responses (request_id, code, message, headers, body) VALUES (?, ?, ?, ?, ?)`,
		reqID, rr.Response.Code, rr.Response.Message, respHeadersJSON, rr.Response.Body)
	if err != nil {
		tx.Rollback()
		return err
	}
	log.Println("All added successfully")
	return tx.Commit()
}

func GetRequestResponseByID(id int) (dto.RequestAndResponse, error) {
	var rr dto.RequestAndResponse

	// Чтение запроса
	row := db.QueryRow(`SELECT id, method, path, get_params, headers, cookie, body, secure, rerequest FROM requests WHERE id = ?`, id)

	var getParamsJSON, headersJSON, cookieJSON string
	err := row.Scan(
		&rr.Request.ID,
		&rr.Request.Method,
		&rr.Request.Path,
		&getParamsJSON,
		&headersJSON,
		&cookieJSON,
		&rr.Request.Body,
		&rr.Request.Secure,
		&rr.Request.FastRerequests,
	)
	if err != nil {
		return rr, err
	}

	// Распаковка JSON полей
	_ = json.Unmarshal([]byte(getParamsJSON), &rr.Request.GetParams)
	_ = json.Unmarshal([]byte(headersJSON), &rr.Request.Headers)
	_ = json.Unmarshal([]byte(cookieJSON), &rr.Request.Cookie)

	// Чтение ответа
	row = db.QueryRow(`SELECT id, code, message, headers, body FROM responses WHERE request_id = ?`, id)

	var respHeadersJSON string
	err = row.Scan(
		&rr.Response.ID,
		&rr.Response.Code,
		&rr.Response.Message,
		&respHeadersJSON,
		&rr.Response.Body,
	)
	if err != nil {
		return rr, err
	}

	_ = json.Unmarshal([]byte(respHeadersJSON), &rr.Response.Headers)

	return rr, nil
}

func GetAllRequests() ([]dto.RequestAndResponse, error) {
	rows, err := db.Query(`
        SELECT r.id, r.method, r.path, r.get_params, r.headers, r.cookie, r.body, r.secure, r.rerequest,
               s.code, s.message, s.headers, s.body
        FROM requests r
        JOIN responses s ON s.request_id = r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []dto.RequestAndResponse
	for rows.Next() {
		var rr dto.RequestAndResponse
		var getParams, reqHeaders, cookie, respHeaders []byte

		err := rows.Scan(&rr.Request.ID, &rr.Request.Method, &rr.Request.Path, &getParams, &reqHeaders, &cookie, &rr.Request.Body, &rr.Request.Secure, &rr.Request.FastRerequests,
			&rr.Response.Code, &rr.Response.Message, &respHeaders, &rr.Response.Body)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(getParams, &rr.Request.GetParams)
		json.Unmarshal(reqHeaders, &rr.Request.Headers)
		json.Unmarshal(cookie, &rr.Request.Cookie)
		json.Unmarshal(respHeaders, &rr.Response.Headers)

		result = append(result, rr)
	}
	log.Println("Responses read successfully")
	return result, nil
}
