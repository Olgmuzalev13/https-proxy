package db

import (
    "database/sql"
    "encoding/json"
    "log"
	"httpproxy/dto"
    _ "modernc.org/sqlite"
)

var db *sql.DB

func initDB() {
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
            body TEXT
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

    res, err := tx.Exec(`INSERT INTO requests (method, path, get_params, headers, cookie, body) VALUES (?, ?, ?, ?, ?, ?)`,
        rr.Request.Method, rr.Request.Path, getParamsJSON, headersJSON, cookieJSON, rr.Request.Body)
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

    return tx.Commit()
}

func GetAllRequests() ([]dto.RequestAndResponse, error) {
    rows, err := db.Query(`
        SELECT r.id, r.method, r.path, r.get_params, r.headers, r.cookie, r.body,
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

        err := rows.Scan(&rr.Request.ID, &rr.Request.Method, &rr.Request.Path, &getParams, &reqHeaders, &cookie, &rr.Request.Body,
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

    return result, nil
}