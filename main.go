package main

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	_ "dm"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"mime/quotedprintable"
	"net/smtp"
	"net/textproto"
	"os"

	"github.com/robfig/cron/v3"
	"github.com/xuri/excelize/v2"
)

/**
Configuration file example:

	{
		"email": {
			"host": "smtp.xxxx.com",
			"port": 587,
			"username": "USERNAME",
			"password": "PASSWORD"
		},
		"db": {
			"host": "xxx.xxx.xxx.xxx",
			"port": 5236,
			"username": "USERNAME",
			"password": "PASSWORD"
		},
		"post": [
			{
				"from": "FROM_EMAIL",
				"to": ["TO_EMAIL"],
				"subject": "SUBJECT",
				"body": "BODY",
				"attachment": [
					{
						"table": "TABLE_NAME",
						"excel": "EXCEL_FILE_NAME"
					}
				]
			}
		],
		"time": "0 0 0 * * *"
	}
*/

// Attachment represents an email attachment.
type Attachment struct {
	fileName string
	mimeType string
	file     *bytes.Buffer
}

// Config represents the configuration of the application.
type Config struct {
	Email EmailConfig  `json:"email"`
	DB    DBConfig     `json:"db"`
	Post  []PostConfig `json:"post"`
	Time  string       `json:"time"`
}

// EmailConfig represents the email configuration.
type EmailConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// DBConfig represents the database configuration.
type DBConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// PostConfig represents the email post configuration.
type PostConfig struct {
	From       string                  `json:"from"`
	To         []string                `json:"to"`
	Subject    string                  `json:"subject"`
	Body       string                  `json:"body"`
	Attachment []TableAttachmentConfig `json:"attachment"`
}

// TableAttachmentConfig represents the table attachment configuration.
type TableAttachmentConfig struct {
	Table string `json:"table"`
	Excel string `json:"excel"`
}

// writeBody writes the email body to the multipart writer.
//
// @param writer: multipart writer
// @param body: email body
// @return error: error if any
func writeBody(writer *multipart.Writer, body string) error {
	log.Println("Writing email body...")

	// Create a new MIME part for the email body
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"text/plain; charset=utf-8"},
		"Content-Transfer-Encoding": {"quoted-printable"},
	})
	if err != nil {
		log.Printf("Failed to create MIME part for email body: %v", err)
		return err
	}

	// Create a new quoted-printable writer
	qp := quotedprintable.NewWriter(part)
	defer qp.Close() // Ensure qp is closed on function return

	// Write the email body to the part
	if _, err = qp.Write([]byte(body)); err != nil {
		log.Printf("Failed to write email body: %v", err)
		return err
	}

	log.Println("Email body written successfully.")
	return nil
}

// writeAttachment writes the attachment to the multipart writer.
//
// @param writer: multipart writer
// @param attachment: attachment buffer
// @param fileName: attachment file name
// @param mimeType: attachment MIME type
// @return error: error if any
func writeAttachment(writer *multipart.Writer, attachment *bytes.Buffer, fileName, mimeType string) error {
	log.Printf("Writing email attachment: %s...", fileName)

	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {mimeType},
		"Content-Transfer-Encoding": {"base64"},
		"Content-Disposition":       {fmt.Sprintf(`attachment; filename="%s"`, fileName)},
	})
	if err != nil {
		log.Printf("Failed to create MIME part for attachment: %v", err)
		return err
	}

	encoder := base64.NewEncoder(base64.StdEncoding, part)
	defer encoder.Close()

	_, err = io.Copy(encoder, attachment)
	if err != nil {
		log.Printf("Failed to write attachment: %v", err)
		return err
	}

	log.Printf("Attachment %s written successfully.", fileName)
	return nil
}

// SendEmail sends an email with attachments.
//
// @param smtpServer: SMTP server address
// @param port: SMTP server port
// @param username: SMTP server username
// @param password: SMTP server password
// @param from: email sender
// @param to: email recipient
// @param subject: email subject
// @param body: email body
// @param attachments: email attachments
// @return error: error if any
func SendEmail(
	smtpServer string,
	port string,
	username string,
	password string,
	from string,
	to string,
	subject string,
	body string,
	attachments []Attachment) error {

	log.Printf("Starting to prepare email to: %s", to)
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	defer writer.Close()

	headers := map[string]string{
		"From":         from,
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": fmt.Sprintf("multipart/mixed; boundary=%s", writer.Boundary()),
	}
	for key, value := range headers {
		buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	buf.WriteString("\r\n")

	if err := writeBody(writer, body); err != nil {
		log.Printf("Failed to write email body: %v", err)
		return err
	}

	for _, attachment := range attachments {
		if err := writeAttachment(writer, attachment.file, attachment.fileName, attachment.mimeType); err != nil {
			log.Printf("Failed to write attachment: %v", err)
			return err
		}
	}

	serverAddress := fmt.Sprintf("%s:%s", smtpServer, port)
	conn, err := tls.Dial("tcp", serverAddress, &tls.Config{InsecureSkipVerify: false})
	if err != nil {
		log.Printf("Failed to connect to SMTP server: %v", err)
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, smtpServer)
	if err != nil {
		log.Printf("Failed to create SMTP client: %v", err)
		return err
	}
	defer client.Close()

	auth := smtp.PlainAuth("", username, password, smtpServer)
	if err = client.Auth(auth); err != nil {
		log.Printf("SMTP authentication failed: %v", err)
		return err
	}

	if err = client.Mail(from); err != nil {
		log.Printf("Failed to set sender: %v", err)
		return err
	}
	if err = client.Rcpt(to); err != nil {
		log.Printf("Failed to set recipient: %v", err)
		return err
	}

	writerClient, err := client.Data()
	if err != nil {
		log.Printf("Failed to start email data transfer: %v", err)
		return err
	}

	if _, err = writerClient.Write(buf.Bytes()); err != nil {
		log.Printf("Failed to send email data: %v", err)
		return err
	}
	log.Printf("Successfully sent email to: %s", to)

	return writerClient.Close()
}

// exportTableToExcel exports a table from the database to an Excel file.
//
// @param db: database connection
// @param tableName: table name to export
// @return *bytes.Buffer: Excel file buffer
// @return error: error if any
func exportTableToExcel(db *sql.DB, tableName string) (*bytes.Buffer, error) {
	log.Printf("Starting to export table %s to Excel", tableName)

	file := excelize.NewFile()
	sheetName := "Sheet1"
	index, err := file.NewSheet(sheetName)
	if err != nil {
		log.Printf("Failed to create Excel sheet: %v", err)
		return nil, err
	}

	query := fmt.Sprintf("SELECT * FROM %s", tableName)
	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Failed to query table %s: %v", tableName, err)
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		log.Printf("Failed to get columns from table %s: %v", tableName, err)
		return nil, err
	}

	for i, colName := range columns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		file.SetCellValue(sheetName, cell, colName)
	}

	values := make([]sql.RawBytes, len(columns))
	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	rowNum := 2
	for rows.Next() {
		err = rows.Scan(scanArgs...)
		if err != nil {
			log.Printf("Failed to scan row in table %s: %v", tableName, err)
			return nil, err
		}
		for colNum, value := range values {
			cell, _ := excelize.CoordinatesToCellName(colNum+1, rowNum)
			if value == nil {
				file.SetCellValue(sheetName, cell, "NULL")
			} else {
				file.SetCellValue(sheetName, cell, string(value))
			}
		}
		rowNum++
	}

	if err = rows.Err(); err != nil {
		log.Printf("Error during row iteration for table %s: %v", tableName, err)
		return nil, err
	}

	file.SetActiveSheet(index)

	buffer := new(bytes.Buffer)
	if err := file.Write(buffer); err != nil {
		log.Printf("Failed to write Excel file to buffer: %v", err)
		return nil, err
	}

	if err := file.Close(); err != nil {
		log.Printf("Failed to close Excel file: %v", err)
		return nil, err
	}

	log.Printf("Successfully exported table %s to Excel", tableName)
	return buffer, nil
}

// createDMDB creates a connection to the DM database.
//
// @param username: database username
// @param password: database password
// @param host: database host
// @param port: database port
// @return *sql.DB: database connection
// @return error: error if any
func createDMDB(username string, password string, host string, port string) (*sql.DB, error) {
	log.Println("Attempting to connect to the DM database...")

	if username == "" || password == "" || host == "" || port == "" {
		err := fmt.Errorf("invalid database credentials or host information")
		log.Printf("Failed to connect: %v", err)
		return nil, err
	}

	dataSourceName := fmt.Sprintf("dm://%s:%s@%s:%s", username, password, host, port)

	db, err := sql.Open("dm", dataSourceName)
	if err != nil {
		log.Printf("Failed to open database connection: %v", err)
		return nil, err
	}

	if err := db.Ping(); err != nil {
		log.Printf("Failed to ping database: %v", err)
		db.Close()
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(0)

	log.Println("DM database connection established successfully.")
	return db, nil
}

// readConfig reads the configuration from the given file path.
//
// @param configPath: configuration file path
// @return *Config: configuration
func readConfig(configPath string) (*Config, error) {
	log.Printf("Reading configuration from: %s", configPath)

	file, err := os.Open(configPath)
	if err != nil {
		log.Printf("Failed to open config file: %v", err)
		return nil, err
	}
	defer file.Close()

	var config Config

	decoder := json.NewDecoder(file)
	if err = decoder.Decode(&config); err != nil {
		log.Printf("Failed to decode config file: %v", err)
		return nil, err
	}

	log.Println("Configuration file read successfully.")
	return &config, nil
}

// task is the main task that sends emails with attachments.
//
// @param config: configuration
func task(config Config) {
	log.Println("Starting task...")

	db, err := createDMDB(config.DB.Username, config.DB.Password, config.DB.Host, fmt.Sprintf("%d", config.DB.Port))
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return
	}
	defer db.Close()

	for _, post := range config.Post {
		attachments := make([]Attachment, 0)
		for _, attachmentConfig := range post.Attachment {
			attachment, err := exportTableToExcel(db, attachmentConfig.Table)
			if err != nil {
				log.Printf("Failed to export table %s to Excel: %v", attachmentConfig.Table, err)
				return
			}

			attachments = append(attachments, Attachment{
				fileName: attachmentConfig.Excel,
				mimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				file:     attachment,
			})
		}

		for _, recipient := range post.To {
			err := SendEmail(
				config.Email.Host,
				fmt.Sprintf("%d", config.Email.Port),
				config.Email.Username,
				config.Email.Password,
				post.From,
				recipient,
				post.Subject,
				post.Body,
				attachments,
			)

			if err != nil {
				log.Printf("Failed to send email to %s: %v", recipient, err)
				return
			}

			log.Printf("Email sent to %s successfully", recipient)
		}
	}

	log.Println("Task completed successfully.")
}

// main is the entry point of the application.
func main() {
	configPath := flag.String("config", "", "json config file path")
	flag.Parse()

	if *configPath == "" {
		log.Println("Config file path is empty")
		return
	}

	config, err := readConfig(*configPath)
	if err != nil {
		log.Printf("Failed to read config file: %v", err)
		return
	}

	log.Println("Configuration loaded successfully")

	c := cron.New()
	_, err = c.AddFunc(config.Time, func() {
		task(*config)
	})

	if err != nil {
		log.Printf("Failed to add cron job: %v", err)
		return
	}

	c.Start()

	select {}
}
