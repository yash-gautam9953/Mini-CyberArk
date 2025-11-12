package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var mongoClient *mongo.Client
var credColl *mongo.Collection

type Credential struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	Username    string             `bson:"username" json:"username"`
	Password    string             `bson:"password" json:"password"`
	LastRotated time.Time          `bson:"last_rotated" json:"last_rotated,omitempty"`
}

type RetrieveRequest struct {
	Reason string `json:"reason"`
}

func main() {
	// Seed random generator
	rand.Seed(time.Now().UnixNano())

	// MongoDB connection
	// You can change URI/db/collection via environment variables if desired.
	mongoURI := "mongodb://localhost:27017"
	dbname := "miniCyberarkVault"
	collName := "credentials"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	mongoClient, err = mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("MongoDB connection error: %v", err)
	}
	if err = mongoClient.Ping(ctx, nil); err != nil {
		log.Fatalf("MongoDB ping failed: %v", err)
	}
	defer func() {
		_ = mongoClient.Disconnect(context.Background())
	}()

	credColl = mongoClient.Database(dbname).Collection(collName)

	// Ensure unique index on username
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "username", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("uniq_username"),
	}
	if _, err := credColl.Indexes().CreateOne(ctx, indexModel); err != nil {
		log.Fatalf("Index creation failed: %v", err)
	}

	// Routes
	http.HandleFunc("/create", createCredential)
	http.HandleFunc("/retrieve/", retrieveCredential)
	http.HandleFunc("/health", healthCheck)

	log.Println("🚀 Server Running at PORT : 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// Generate a random password
func generateRandomPassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	length := 12
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// Create credential
func createCredential(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var cred Credential
	if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if cred.Username == "" || cred.Password == "" {
		http.Error(w, "Username and password required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	cred.LastRotated = time.Now()
	if _, err := credColl.InsertOne(ctx, cred); err != nil {
		// Handle duplicate username
		var we mongo.WriteException
		if errors.As(err, &we) {
			// 11000 is duplicate key
			for _, e := range we.WriteErrors {
				if e.Code == 11000 {
					http.Error(w, "Username already exists", http.StatusConflict)
					return
				}
			}
		}
		http.Error(w, "Insert failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"username": cred.Username,
		"password": cred.Password,
		"message":  "Credential created successfully",
	})
}

// Retrieve and rotate password
func retrieveCredential(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimPrefix(r.URL.Path, "/retrieve/")
	if username == "" {
		http.Error(w, "Username is required in path", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var cred Credential
	if err := credColl.FindOne(ctx, bson.M{"username": username}).Decode(&cred); err != nil {
		if err == mongo.ErrNoDocuments {
			http.Error(w, "Credential not found", http.StatusNotFound)
		} else {
			log.Printf("Mongo error for username '%s': %v", username, err)
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	// Capture current password and return it immediately
	currentPassword := cred.Password

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"username":       username,
		"password":       currentPassword,
		"retrieved_time": time.Now().Format("2006-01-02 15:04:05 MST"),
	})

	// Asynchronously rotate the password after 10 seconds (background)
	newPassword := generateRandomPassword()
	go func(un string, npw string) {
		time.Sleep(10 * time.Second)

		istLoc, _ := time.LoadLocation("Asia/Kolkata")
		updatedTime := time.Now().In(istLoc)

		ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel2()

		update := bson.M{"$set": bson.M{
			"password":     npw,
			"last_rotated": updatedTime,
		}}
		if _, err := credColl.UpdateOne(ctx2, bson.M{"username": un}, update); err != nil {
			log.Printf("Delayed update failed for username '%s': %v", un, err)
			return
		}
		log.Printf("Password for '%s' rotated at %s", un, updatedTime.Format("2006-01-02 15:04:05 MST"))
	}(username, newPassword)
}

// Health check endpoint
func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})	

}
