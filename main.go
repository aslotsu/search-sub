package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
	"github.com/typesense/typesense-go/typesense"
)

type Post struct {
	Id            string    `json:"id"`
	UserId        string    `json:"user_id"`
	Username      string    `json:"username"`
	UserPicture   string    `json:"user_picture"`
	UserBio       string    `json:"user_bio"`
	UserProgramme string    `json:"user_programme"`
	UserYear      int       `json:"user_year"`
	UserCampus    string    `json:"user_campus"`
	Subject       string    `json:"subject"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	Images        []string  `json:"images"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type User struct {
	Id          string    `json:"id"`
	Username    string    `json:"username"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	Bio         string    `json:"bio"`
	Picture     string    `json:"picture"`
	School      string    `json:"school"`
	Country     string    `json:"country"`
	Campus      string    `json:"campus"`
	InfoUpdated bool      `json:"info_updated"`
	Program     string    `json:"program"`
	Year        int       `json:"year"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SubscriberService struct {
	natsConn      *nats.Conn
	usersClient   *typesense.Client
	postsClient   *typesense.Client
}

func writeCredsFileFromEnv() string {
	creds := os.Getenv("NATS_CREDS")
	if creds == "" {
		log.Fatal("NATS_CREDS env var is not set")
	}
	tmpfile, err := os.CreateTemp("", "nats-user-*.creds")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := tmpfile.Write([]byte(creds)); err != nil {
		log.Fatal(err)
	}
	tmpfile.Close()
	return tmpfile.Name()
}

func main() {
	start := time.Now()
	_ = godotenv.Load()

	// Connect to NATS
	credsPath := writeCredsFileFromEnv()
	nc, err := nats.Connect(`connect.ngs.global`, nats.UserCredentials(credsPath))
	if err != nil {
		log.Fatalf("failed to connect to NATS: %v", err)
	}
	defer nc.Drain()
	log.Println("NATS connection time:", time.Since(start))

	// Connect to Typesense instances
	usersClient := typesense.NewClient(
		typesense.WithServer("https://users2.exobook.ca:8108"),
		typesense.WithAPIKey(os.Getenv(
			"TYPESENSE_API_KEY",
		)),
	)

	postsClient := typesense.NewClient(
		typesense.WithServer("https://posts2.exobook.ca:8108"), // Adjust as needed
		typesense.WithAPIKey(os.Getenv(
			"TYPESENSE_API_KEY",
		)),
	)

	// Create subscriber service
	service := &SubscriberService{
		natsConn:    nc,
		usersClient: usersClient,
		postsClient: postsClient,
	}

	// Subscribe to all events
	service.setupSubscriptions()

	fmt.Println("🚀 Listening for events on NATS...")
	fmt.Println("📝 Subscribed to: users.created, users.updated, users.deleted")
	fmt.Println("📄 Subscribed to: posts.upsert, posts.deleted")
	
	select {} // block forever
}

func (s *SubscriberService) setupSubscriptions() {
	// Users events
	s.subscribeToUserCreated()
	s.subscribeToUserUpdated()
	s.subscribeToUserDeleted()
	
	// Posts events
	s.subscribeToPostUpsert()
	s.subscribeToPostDeleted()
}

func (s *SubscriberService) subscribeToUserCreated() {
	_, err := s.natsConn.Subscribe("users.created", func(msg *nats.Msg) {
		log.Println("📥 New user created")
		
		var user User
		if err := json.Unmarshal(msg.Data, &user); err != nil {
			log.Printf("❌ Failed to unmarshal user: %v", err)
			return
		}

		document := s.userToDocument(user)
		
		_, err := s.usersClient.Collection("users").Documents().Create(context.Background(), document)
		if err != nil {
			log.Printf("❌ Failed to create user in Typesense: %v", err)
			return
		}
		
		log.Printf("✅ Created user in search index: %s (%s)", user.Id, user.Name)
	})
	
	if err != nil {
		log.Fatalf("Failed to subscribe to users.created: %v", err)
	}
}

func (s *SubscriberService) subscribeToUserUpdated() {
	_, err := s.natsConn.Subscribe("users.updated", func(msg *nats.Msg) {
		log.Println("📥 User updated")
		
		var user User
		if err := json.Unmarshal(msg.Data, &user); err != nil {
			log.Printf("❌ Failed to unmarshal user: %v", err)
			return
		}

		document := s.userToDocument(user)
		
		_, err := s.usersClient.Collection("users").Documents().Upsert(context.Background(), document)
		if err != nil {
			log.Printf("❌ Failed to update user in Typesense: %v", err)
			return
		}
		
		log.Printf("✅ Updated user in search index: %s (%s)", user.Id, user.Name)
	})
	
	if err != nil {
		log.Fatalf("Failed to subscribe to users.updated: %v", err)
	}
}

func (s *SubscriberService) subscribeToUserDeleted() {
	_, err := s.natsConn.Subscribe("users.deleted", func(msg *nats.Msg) {
		log.Println("📥 User deleted")
		
		// For deletes, we might just get the ID
		var deleteEvent struct {
			Id string `json:"id"`
		}
		if err := json.Unmarshal(msg.Data, &deleteEvent); err != nil {
			log.Printf("❌ Failed to unmarshal delete event: %v", err)
			return
		}

		_, err := s.usersClient.Collection("users").Document(deleteEvent.Id).Delete(context.Background())
		if err != nil {
			log.Printf("❌ Failed to delete user from Typesense: %v", err)
			return
		}
		
		log.Printf("✅ Deleted user from search index: %s", deleteEvent.Id)
	})
	
	if err != nil {
		log.Fatalf("Failed to subscribe to users.deleted: %v", err)
	}
}

func (s *SubscriberService) subscribeToPostUpsert() {
	_, err := s.natsConn.Subscribe("posts.upsert", func(msg *nats.Msg) {
		log.Println("📥 Post upserted")
		
		var post Post
		if err := json.Unmarshal(msg.Data, &post); err != nil {
			log.Printf("❌ Failed to unmarshal post: %v", err)
			return
		}

		document := s.postToDocument(post)
		
		_, err := s.postsClient.Collection("posts").Documents().Upsert(context.Background(), document)
		if err != nil {
			log.Printf("❌ Failed to upsert post in Typesense: %v", err)
			return
		}
		
		log.Printf("✅ Upserted post in search index: %s", post.Id)
	})
	
	if err != nil {
		log.Fatalf("Failed to subscribe to posts.upsert: %v", err)
	}
}

func (s *SubscriberService) subscribeToPostDeleted() {
	_, err := s.natsConn.Subscribe("posts.deleted", func(msg *nats.Msg) {
		log.Println("📥 Post deleted")
		
		var deleteEvent struct {
			Id string `json:"id"`
		}
		if err := json.Unmarshal(msg.Data, &deleteEvent); err != nil {
			log.Printf("❌ Failed to unmarshal delete event: %v", err)
			return
		}

		_, err := s.postsClient.Collection("posts").Document(deleteEvent.Id).Delete(context.Background())
		if err != nil {
			log.Printf("❌ Failed to delete post from Typesense: %v", err)
			return
		}
		
		log.Printf("✅ Deleted post from search index: %s", deleteEvent.Id)
	})
	
	if err != nil {
		log.Fatalf("Failed to subscribe to posts.deleted: %v", err)
	}
}

// Helper functions to convert structs to Typesense documents
func (s *SubscriberService) userToDocument(user User) map[string]interface{} {
	return map[string]interface{}{
		"id":           user.Id,
		"username":     user.Username,
		"name":         user.Name,
		"email":        user.Email,
		"bio":          user.Bio,
		"picture":      user.Picture,
		"school":       user.School,
		"country":      user.Country,
		"campus":       user.Campus,
		"info_updated": user.InfoUpdated,
		"program":      user.Program,
		"year":         user.Year,
		"created_at":   user.CreatedAt.Unix(),
		"updated_at":   user.UpdatedAt.Unix(),
	}
}

func (s *SubscriberService) postToDocument(post Post) map[string]interface{} {
	return map[string]interface{}{
		"id":             post.Id,
		"user_id":        post.UserId,
		"user_name":      post.Username,
		"user_picture":   post.UserPicture,
		"user_bio":       post.UserBio,
		"user_programme": post.UserProgramme,
		"user_year":      post.UserYear,
		"user_campus":    post.UserCampus,
		"subject":        post.Subject,
		"title":          post.Title,
		"content":        post.Content,
		"images":         post.Images,
		"created_at":     post.CreatedAt.Unix(),
		"updated_at":     post.UpdatedAt.Unix(),
	}
}