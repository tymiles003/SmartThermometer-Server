package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/sheets/v4"
)

var srv *sheets.Service
var rowMap = make(map[int]int)
var colNow int

var lastDate time.Time

func writeTemp(num int, temp float32) {
	_, ok := rowMap[num]
	if !ok {
		log.Println("GSheets Write Temp: Can't find number", num)
		return
	}

	checkDate()
	cur := readCell(colNow, rowMap[num])

	morTemp := "--"
	aftTemp := "--"

	if cur != "" {
		res := strings.Split(cur, "/")
		morTemp, aftTemp = res[0], res[1]
	}

	loc, err := time.LoadLocation(conf.TimeZone)
	if err != nil {
		log.Println("GSheets Time Loc: ", err)
	}

	if time.Now().In(loc).Hour()+1 < conf.Noon {
		err = writeCell(fmt.Sprintf("%.1f", temp)+"/"+aftTemp, colNow, rowMap[num], false)
	} else {
		err = writeCell(morTemp+"/"+fmt.Sprintf("%.1f", temp), colNow, rowMap[num], false)
	}
	if err != nil {
		log.Println("GSheets Write Temp: ", err)
	}
}

func checkDate() {
	if !(lastDate.YearDay() == time.Now().YearDay()) {
		lastDate = time.Now()

		latestDate := readCell(colNow, 1)
		if latestDate == "" {
			getLastCol()
			latestDate = readCell(colNow, 1)
		}

		layout := "1/2"
		t, err := time.Parse(layout, latestDate)
		if err != nil {
			log.Println("GSheets Date: ", err)
		}

		loc, _ := time.LoadLocation(conf.TimeZone)
		curDate := time.Now().In(loc)

		if !(t.Month() == curDate.Month() && t.Day() == curDate.Day()) {
			log.Println("GSheets Date: Getting to the next day")
			colNow++
			err := writeCell(fmt.Sprintf("%d/%d", curDate.Month(), curDate.Day()), colNow, 1, false)
			if err != nil {
				log.Println("GSheets Date: ", err)
			}
		}
	}
}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func readSheet(readRange string) *sheets.ValueRange {
	resp, err := srv.Spreadsheets.Values.Get(conf.SheetsID, readRange).Do()
	if err != nil {
		log.Fatalf("Unable to retrieve data from sheet: %v", err)
		return nil
	}
	if len(resp.Values) == 0 {
		return nil
	}
	return resp
}

func writeCell(val string, col int, row int, raw bool) error {
	var vr sheets.ValueRange

	vr.Values = append(vr.Values, []interface{}{val})

	writeRange := fmt.Sprintf("%s%d:%s%d", toChar(col), row, toChar(col), row)
	var inputOption string
	if raw {
		inputOption = "RAW"
	} else {
		inputOption = "USER_ENTERED"
	}

	_, err := srv.Spreadsheets.Values.Update(conf.SheetsID, writeRange, &vr).ValueInputOption(inputOption).Do()
	return err
}

func toChar(i int) string {
	res := ""
	for i > 0 {
		res = string('A'+(i-1)%26) + res
		i = (i - 1) / 26
	}
	return res
}

func readCell(col int, row int) string {
	read := readSheet(fmt.Sprintf("%s%d:%s%d", toChar(col), row, toChar(col), row))
	if read != nil {
		return read.Values[0][0].(string)
	}
	return ""
}

func authGSheets() {
	b, err := ioutil.ReadFile("credentials.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved token.json.
	config, err := google.ConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets")
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(config)

	srv, err = sheets.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve Sheets client: %v", err)
	}
}

func getLastCol() {
	for r, row := range readSheet("A:B").Values {
		if len(row) > 1 && row[1] != "" {
			num, err := strconv.Atoi(row[1].(string))
			if err != nil {
				log.Println("GSheets Row Map: ", err)
			}
			rowMap[num] = r + 1
		}
	}

	colNow = len(readSheet("1:1").Values[0])
}

func gsheetsInit() {
	log.Println("Connecting to GSheets...")
	authGSheets()

	getLastCol()
	checkDate()
}
