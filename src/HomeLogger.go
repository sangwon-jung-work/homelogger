package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/quhar/bme280"
	"golang.org/x/exp/io/i2c"
)

// some options
const (
	wait_minute      = 5     // time.Sleep interval
	sql_wait_second  = 5     // context sql execute wait timeout
	active_debug_log = false // some debug log handling(insert count, bme280 values)
)

// Insert logger data
type LoggerData struct {
	device_name     string
	temperature     string
	humidity        string
	pressure        string
	raw_temperature string
	raw_humidity    string
	raw_pressure    string
}

// Line notify server response
type MessageResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// Send line notify message(new token required)
func sendLineMsg(message string) bool {
	url := "https://notify-api.line.me/api/notify"

	body := []byte(fmt.Sprintf("message=" + message))

	// Create a HTTP post request
	r, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		log.Printf("Error %s when send line message\n", err)
		return false
	}

	// set line auth token header
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Add("Authorization", "Bearer (your token text))") // new token required

	client := &http.Client{}
	res, err := client.Do(r)
	if err != nil {
		panic(err)
	}

	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {

		bodyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			log.Fatal("error when ReadAll", err)
		}

		bodyString := string(bodyBytes)
		log.Printf("res.Body is %s \n", bodyString)

		rdata := MessageResponse{}
		derr := json.Unmarshal([]byte(bodyString), &rdata)
		if derr != nil {
			log.Printf("Error %s when decode response message\n", derr)
			return false
		}

		log.Printf("Response StatusCode is %d\n", rdata.Status)

		if rdata.Status != http.StatusOK {
			log.Printf("StatusCode %d when response status code\n", rdata.Status)
			return false
		}

	} else {
		log.Printf("StatusCode %d when request message\n", res.StatusCode)
		return false
	}

	return true
}

// database connection handling(datasource url required)
// datasource url detail Documentation see this, https://github.com/golang/go/wiki/SQLDrivers
func dbConnection() (*sql.DB, error) {

	// Create the database handle
	db, err := sql.Open("mysql", "user:password_without_escape@tcp(ip:port)/database?tls=false")
	if err != nil {
		log.Printf("Error %s when opening DB\n", err)
		return nil, err
	}
	//defer db.Close()

	// May vary by driver
	db.SetConnMaxLifetime(0) // lifetime per connections. 0 is infinite lifetime
	db.SetMaxIdleConns(2)    // max idle connections. 2 is default
	db.SetMaxOpenConns(4)    // max open connections. default is 0(unlimited)

	ctx, cancelfunc := context.WithTimeout(context.Background(), sql_wait_second*time.Second)
	defer cancelfunc()
	err = db.PingContext(ctx)
	if err != nil {
		log.Printf("Error %s pinging DB", err)
		return nil, err
	}
	log.Printf("Connected to database successfully!")

	return db, nil
}

// insert sensor data to table
func insertData(db *sql.DB, data LoggerData) error {

	query := `INSERT INTO SENSOR_DATAS (
		temperature,humidity,pressure,
		raw_temperature,raw_humidity,raw_pressure,
		device_hostname
	)
	VALUES (
		?, ?, ?,
		?, ?, ?,
		?
		)`

	ctx, cancelfunc := context.WithTimeout(context.Background(), sql_wait_second*time.Second)
	defer cancelfunc()

	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		log.Printf("Error %s when preparing SQL statement", err)
		return err
	}
	defer stmt.Close()

	// set params for insert query
	res, err := stmt.ExecContext(ctx, data.temperature, data.humidity, data.pressure, data.raw_temperature, data.raw_humidity, data.raw_pressure, data.device_name)
	if err != nil {
		log.Printf("Error %s when inserting row into logger table", err)
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		log.Printf("Error %s when finding rows affected", err)
		return err
	}

	if active_debug_log {
		log.Printf("%d products created ", rows)
	}

	return nil
}

func main() {

	// get connection
	db, err := dbConnection()
	if err != nil {

		if chk := sendLineMsg(fmt.Sprintf("unable to use data source! %s", err)); !chk {
			log.Printf("Errors when sendLineMsg")
		}

		log.Fatal("unable to use data source!", err)
	}
	defer db.Close()

	// open i2c device
	device, err := i2c.Open(&i2c.Devfs{Dev: "/dev/i2c-1"}, bme280.I2CAddr)
	if err != nil {

		if chk := sendLineMsg(fmt.Sprintf("i2c open fail! %s", err)); !chk {
			log.Printf("Errors when sendLineMsg")
		}

		log.Fatal("i2c open fail!", err)
	}

	for {

		// Check if connection is active
		err = db.Ping()
		if err != nil {
			log.Printf("connection is invalid. error msg %s", err)

			if chk := sendLineMsg("connection is invalid. will reconnect."); !chk {
				log.Printf("Errors when sendLineMsg")
			}

			// reconnection
			db, err = dbConnection()
		}

		bme := bme280.New(device)
		err = bme.Init()

		temp, press, hum, err := bme.EnvData()

		if err != nil {
			if chk := sendLineMsg("Error when init bme280"); !chk {
				log.Printf("Errors when sendLineMsg")
			}
			//panic(err)
		}

		if active_debug_log {
			fmt.Printf("Temp: %fC, Press: %fhPa, Hum: %f%%\n", temp, press, hum)
		}

		loggerData := LoggerData{
			device_name:     "somewhere", // sensing space name
			temperature:     fmt.Sprintf("%.2f", temp),
			humidity:        fmt.Sprintf("%.2f", hum),
			pressure:        fmt.Sprintf("%.2f", press),
			raw_temperature: fmt.Sprintf("%.5f", temp),
			raw_humidity:    fmt.Sprintf("%.5f", hum),
			raw_pressure:    fmt.Sprintf("%.5f", press),
		}

		err = insertData(db, loggerData)
		if err != nil {
			log.Printf("Insert logger data failed with error %s", err)

			if chk := sendLineMsg(fmt.Sprintf("Insert logger data failed with error %s", err)); !chk {
				log.Printf("Errors when sendLineMsg")
			}
		}

		time.Sleep(wait_minute * time.Minute)
	} // for loop end

}
