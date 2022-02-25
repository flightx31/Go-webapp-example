package main

import (
	"database/sql"
	"embed"
	"fmt"
	"github.com/gorilla/mux"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/afero"
	"net/http"
	"os"
	"strings"
	"time"
)

//go:embed sql/*
var sqlFiles embed.FS

//go:embed ui/*
var staticFiles embed.FS
var db *sql.DB

const appName = "helloworldapp"

// init() is run by Golang the first time a program is run.
func init() {
	// UNIX Time is faster and smaller than most timestamps
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	// Default level for this example is info, unless debug flag is present
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Info().Msg("Running init function")
	startup()
}

func startup() {
	dirname, err := os.UserHomeDir()
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	var AppFs = afero.NewOsFs()
	var dbFilePath = dirname + afero.FilePathSeparator + appName

	_, err = AppFs.Stat(dbFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info().Msg("Creating directory: " + dbFilePath)
			AppFs.MkdirAll(dbFilePath, 0754)
		} else {
			log.Error().Err(err).Msg("")
		}
	}

	var dbFile = dbFilePath + afero.FilePathSeparator + appName + ".db"
	db, err := sql.Open("sqlite3", dbFile)

	if err != nil {
		log.Error().Err(err).Msg("")
	}

	defer db.Close()
	initDatabase(db)
}

func main() {
	server()
}

func server() {
	log.Info().Msg("Configuring server")
	myRouter := mux.NewRouter().StrictSlash(true)

	loggingRouter := loggingMiddleware(myRouter)

	myRouter.HandleFunc("/helloworld", helloWorldHandler).Methods(http.MethodGet)
	myRouter.HandleFunc("/hellovars/{var1}/{var2}", helloVarsHandler).Methods(http.MethodGet)

	// Note: the index, and static files handlers need to be after the routes because they are more generic routes.
	myRouter.HandleFunc("/", homePageHandler)
	fileServer := http.FileServer(http.FS(staticFiles))
	myRouter.PathPrefix("/").Handler(fileServer)

	log.Info().Msg("Starting server")
	srv := &http.Server{
		Handler: loggingRouter,
		Addr:    "127.0.0.1:8081",
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
	err := srv.ListenAndServe()
	if err != nil {
		log.Error().Err(err).Msg("")
	}

}

// *********************************************************
// Middleware
// *********************************************************

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setupCorsResponse(&w)
		if len(r.URL.Path) > 1 {
			r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
		}
		// Do stuff here
		log.Info().Msg("Incomming request to \"" + r.RequestURI + "\"")
		// Call the next handler, which can be another middleware in the chain, or the final handler.
		next.ServeHTTP(w, r)
	})
}

func setupCorsResponse(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	(*w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Authorization")
}

// *********************************************************
// Route Handlers
// *********************************************************

func homePageHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	indexFile := getStaticFileText("ui/index.html")
	w.Write([]byte(indexFile))
}

func helloWorldHandler(w http.ResponseWriter, r *http.Request) {
	indexFile := getStaticFileText("ui/pages/helloworld.html")
	w.Write([]byte(indexFile))
}

func helloVarsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Path params: %v %v\n", vars["var1"], vars["var2"])
}

// *********************************************************
// Database
// *********************************************************

func initDatabase(db *sql.DB) {
	log.Info().Msg("==================================")
	log.Info().Msg("Pinging database")
	err := db.Ping()
	if err != nil {
		log.Error().Err(err).Msg("")
	}
	dbVersion := getCurrentDBVersion(db)

	if dbVersion == -1 {
		log.Info().Msg("No \"version\" table.")
		executeScript(db, getSqlFileText("sql/init.sql"), "Version table init script (version 0)")
		dbVersion = getCurrentDBVersion(db)
	}

	if dbVersion == 0 {
		executeScript(db, getSqlFileText("sql/v1.sql"), "Version 1 script")
		dbVersion = getCurrentDBVersion(db)
	}

	log.Info().Msg("Current database version: " + string(dbVersion))

	// Note: the version of sqlite3 that this library is using does not support running scripts (multiple queries in one execute statement) The below only runs the first query:
	//executeSingleStatement(db, "insert into version (version) values (0);insert into version (version) values (1);")

	log.Info().Msg("==================================")
	log.Info().Msg("")
}

/**
Note: The sql script can only contain sql statements (no comments) and each comment must end with a semicolon.
*/
func executeScript(db *sql.DB, scriptText string, scriptName string) {
	err := executeSingleStatement(db, "BEGIN TRANSACTION;")

	if err != nil {
		log.Error().Err(err)
	}

	log.Info().Msg("Executing script: " + scriptName)
	commands := strings.Split(scriptText, ";")

	for c := 0; c < len(commands); c++ {
		command := commands[c]
		command = strings.Trim(command, " ")
		command = strings.ReplaceAll(command, "\n", "")

		if len(command) > 0 {
			err := executeSingleStatement(db, command)

			if err != nil {
				executeSingleStatement(db, "ROLLBACK;")
				log.Error().Err(err).Msg("")
			}
		}
	}

	executeSingleStatement(db, "COMMIT;")
}

func executeSingleStatement(db *sql.DB, query string) error {

	statement, err := db.Prepare(query)

	if err != nil {
		return err
	}

	_, err = statement.Exec()

	if err != nil {
		return err
	}

	return err
}

func getCurrentDBVersion(db *sql.DB) int64 {
	rows, err := db.Query("select max(version) as version from version")

	if err != nil {
		if err.Error() == "no such table: version" {
			return -1
		}

		log.Error().Err(err).Msg("")
	}

	version := Version{Version: -1}

	rows.Next()
	err = rows.Scan(&version.Version)

	if err != nil {
		rows.Close()
		log.Error().Err(err).Msg("")
	}

	rows.Close()
	return version.Version
}

func getSqlFileText(path string) string {
	data, err := sqlFiles.ReadFile(path)
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	return string(data)
}

func getStaticFileText(path string) string {
	data, err := staticFiles.ReadFile(path)
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	return string(data)
}

// *********************************************************
// Structs
// *********************************************************

type Version struct {
	Version int64 `json:"version"`
}
