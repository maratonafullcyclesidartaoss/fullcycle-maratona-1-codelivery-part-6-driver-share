package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	_ "github.com/sidartaoss/maratona-fullcycle-1/driver/docs"
	httpSwagger "github.com/swaggo/http-swagger"
	_ "github.com/swaggo/swag/example/celler/httputil"
)

type Driver struct {
	Uuid string `json:"uuid" example:"127a2285-0fd8-4df9-b4eb-485292f3e345"`
	Name string `json:"name" example:"Sidarta S"`
}

type Drivers struct {
	Drivers []Driver
}

func init() {
	err := godotenv.Load()
	if err != nil {
		panic("Error loading .env file")
	}
}

func loadDrivers() []byte {
	jsonFile, err := os.Open("drivers.json")
	if err != nil {
		panic(err.Error())
	}

	defer jsonFile.Close()

	data, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		panic(err.Error())
	}
	return data
}

// ListDrivers godoc
// @Summary      List drivers
// @Description  List all drivers
// @Tags		 drivers
// @Accept       json
// @Produce      json
// @Success      200       {array}   Driver
// @Failure      400
// @Failure      500
// @Router       /drivers [get]
func ListDrivers(w http.ResponseWriter, r *http.Request) {
	data := loadDrivers()

	var drivers Drivers
	if err := json.Unmarshal(data, &drivers); err != nil {
		panic(err.Error())
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(drivers)
}

// GetDriver godoc
// @Summary      Get a driver
// @Description	 Get a driver by ID (uuid)
// @Tags		 drivers
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "driver ID" Format(uuid)
// @Success      200  {object}  Driver
// @Failure      400
// @Failure      404
// @Failure      500
// @Router       /drivers/{id} [get]
func GetDriver(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	data := loadDrivers()

	var drivers Drivers
	if err := json.Unmarshal(data, &drivers); err != nil {
		panic(err.Error())
	}

	for _, v := range drivers.Drivers {
		if v.Uuid == vars["id"] {
			driver, err := json.Marshal(v)
			if err != nil {
				panic(err.Error())
			}
			w.Write(driver)
		}
	}
}

// @title           Maratona Full Cycle Driver API
// @version         1.0
// @description     Driver API
// @termsOfService  http://swagger.io/terms/

// @contact.name 	Sidarta Silva
// @contact.url		http://www.sidartaoss.com
// @contact.email	atendimento@sidartaoss.com

// @license.name   Full Cycle License
// @license.url    http://www.fullcycle.com.br

// @host      host.docker.internal:8081
// @BasePath  /
func main() {
	log.Println("alive")

	r := chi.NewRouter()

	r.Use(middleware.Logger)

	r.Get("/drivers", ListDrivers)

	r.Get("/drivers/{id}", GetDriver)

	u := "http://" + os.Getenv("SWAGGER_HOST") + ":" + os.Getenv("SWAGGER_PORT") + "/docs/doc.json"

	r.Get("/docs/*", httpSwagger.Handler(httpSwagger.URL(u)))

	http.ListenAndServe(":8081", r)
}
