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
  discord_url_app  = ""    // discord webhook url
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

// discord message Request json
type MessageRequest struct {
	Content  string `json:"content,omitempty"`
	Username string `json:"username,omitempty"`
}

// Send notify message (Discord)
func sendNotify(servicename string, message string) bool {
	body := new(bytes.Buffer)

	sendData := MessageRequest{
		Content:  message,
		Username: servicename,
	}

	err := json.NewEncoder(body).Encode(sendData)
	if err != nil {
		log.Printf("Error %s when send message\n", err)
		return false
	}

	// Create a HTTP post request
	response, err := http.Post(discord_url_app, "application/json", body)
	if err != nil {
		log.Printf("Error %s when send message\n", err)
		return false
	}

	// if response non 200, 204
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		defer response.Body.Close()

		bodyBytes, err := io.ReadAll(response.Body)
		if err != nil {
			log.Fatal("error when ReadAll", err)
		}

		bodyString := string(bodyBytes)
		log.Printf("Response not 200 or 204. server return %s \n", bodyString)

		return false

	} else {
		// if response 200, 204 (normal)
		log.Printf("StatusCode %d when request message\n", response.StatusCode)
	}

	return true
}

// database connection handling(datasource url required)
// datasource url detail Documentation see this, https://github.com/golang/go/wiki/SQLDrivers
func dbConnection() (*sql.DB, error) {

	// Create the database handle
	db, err := sql.Open("mysql", "user:password_without_escape@tcp(ip:port)/database?tls=false")
	if err != nil {
		log.Printf("Fail sql.Open : %s", err)

		if chk := sendNotify("homelogger", fmt.Sprintf("Fail sql.Open : %s", err)); !chk {
			log.Printf("Errors when sql Open sendNotify")
		}
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
		log.Printf("Fail db.PingContext : %s", err)

		if chk := sendNotify("homelogger", fmt.Sprintf("Fail db.PingContext : %s", err)); !chk {
			log.Printf("Errors when PingContext sendNotify")
		}
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
		log.Printf("Error PrepareContext : %s", err)
		return err
	}
	defer stmt.Close()

	// set params for insert query
	res, err := stmt.ExecContext(ctx, data.temperature, data.humidity, data.pressure, data.raw_temperature, data.raw_humidity, data.raw_pressure, data.device_name)
	if err != nil {
		log.Printf("Error ExecContext : %s", err)
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		log.Printf("Error RowsAffected : %s", err)
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
		log.Fatal("Fail dbConnection!", err)
	}
	defer db.Close()

	// open i2c device
	device, err := i2c.Open(&i2c.Devfs{Dev: "/dev/i2c-1"}, bme280.I2CAddr)
	if err != nil {

		if chk := sendNotify("homelogger", fmt.Sprintf("i2c open fail! %s", err)); !chk {
			log.Printf("Errors when i2c Open sendNotify")
		}

		log.Fatal("i2c open fail!", err)
	}

	for {

		// Check if connection is active
		err = db.Ping()
		if err != nil {
			log.Printf("Fail db.Ping : %s", err)

			if chk := sendNotify("homelogger", "Fail db.Ping. will reconnect."); !chk {
				log.Printf("Errors when db Ping sendNotify")
			}

			// reconnection
			db, err = dbConnection()
		}

		bme := bme280.New(device)
		err = bme.Init()

		temp, press, hum, err := bme.EnvData()

		if err != nil {
			if chk := sendNotify("homelogger", "Error when init bme280"); !chk {
				log.Printf("Errors when bme EnvData sendNotify")
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
			log.Printf("Fail insertData : %s", err)

			if chk := sendNotify("homelogger", fmt.Sprintf("Fail insertData : %s", err)); !chk {
				log.Printf("Errors when insertData sendNotify")
			}
		}

		time.Sleep(wait_minute * time.Minute)
	} // for loop end

}
