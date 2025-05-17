package main

import (
	"os"
	"log"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)
 
func main() {
	
	godotenv.Load()

	portString := os.Getenv("PORT") //to read port
	if portString == "" {
		log.Fatal("PORT isn't found in the environment") //exits program immediately
	}
	
	router := chi.NewRouter() //creates a new HTTP server using chi router package, supports middleware chaining, nested routes, lightweight (like express.js)




	/*MIDDLEWARES- like interceptors that sit between request and your actual logic, modifies requests etc

	they process the request directly or passes it to next handler in chain

	use cases- logging, authentication, error handling, CORS(set headers to allow cross origin requests), rate limiting, compression
	*/

	// CORS(cross origin resource sharing) middleware config,
	// e.g- frontend(Port-3000) can call backend(Port - 8080), differnet origins
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	v1Router := chi.NewRouter()
	v1Router.Get("/healthz", handlerReadiness)
	v1Router.Get("/err", handlerErr)

	router.Mount("/v1", v1Router)



	// server setup, initialize new http server with your router as the handler
	srv := &http.Server{
		Handler: router,
		Addr:    ":" + portString,
	}

	log.Printf("Server starting on port %v", portString)
	err := srv.ListenAndServe() //actually starts the server and begin listening for requests
	if err != nil {
		log.Fatal(err)
	}
}