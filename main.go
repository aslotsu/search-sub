package main

import (
	"context"
	"encoding/json"
	"log/slog"
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
	UserYear      int32     `json:"user_year"`
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
	Year        int32     `json:"year"`
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
		slog.Error("NATS_CREDS env var is not set")
		os.Exit(1)
	}
	tmpfile, err := os.CreateTemp("", "nats-user-*.creds")
	if err != nil {
		slog.Error("failed to create temp creds file", "error", err)
		os.Exit(1)
	}
	if _, err := tmpfile.Write([]byte(creds)); err != nil {
		slog.Error("failed to write creds file", "error", err)
		os.Exit(1)
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
		slog.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Drain()
	slog.Info("NATS connected", "elapsed", time.Since(start))

	// Connect to Typesense instances
	usersClient := typesense.NewClient(
		typesense.WithServer("https://users2.exobook.ca"),
		typesense.WithAPIKey(os.Getenv(
			"TYPESENSE_API_KEY",
		)),
	)

	postsClient := typesense.NewClient(
		typesense.WithServer("https://posts2.exobook.ca"), // Adjust as needed
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

	slog.Info("listening for events on NATS")
	slog.Info("subscribed to subjects", "subjects", []string{"users.created", "users.updated", "users.deleted", "posts.upsert", "posts.deleted"})
	
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
		slog.Info("new user created event received")

		var user User
		if err := json.Unmarshal(msg.Data, &user); err != nil {
			slog.Error("failed to unmarshal user", "error", err)
			return
		}

		document := s.userToDocument(user)

		_, err := s.usersClient.Collection("users").Documents().Create(context.Background(), document)
		if err != nil {
			slog.Error("failed to create user in Typesense", "error", err)
			return
		}

		slog.Info("created user in search index", "id", user.Id, "name", user.Name)
	})

	if err != nil {
		slog.Error("failed to subscribe to users.created", "error", err)
		os.Exit(1)
	}
}

func (s *SubscriberService) subscribeToUserUpdated() {
	_, err := s.natsConn.Subscribe("users.updated", func(msg *nats.Msg) {
		slog.Info("user updated event received")

		var user User
		if err := json.Unmarshal(msg.Data, &user); err != nil {
			slog.Error("failed to unmarshal user", "error", err)
			return
		}

		document := s.userToDocument(user)

		_, err := s.usersClient.Collection("users").Documents().Upsert(context.Background(), document)
		if err != nil {
			slog.Error("failed to update user in Typesense", "error", err)
			return
		}

		slog.Info("updated user in search index", "id", user.Id, "name", user.Name)
	})

	if err != nil {
		slog.Error("failed to subscribe to users.updated", "error", err)
		os.Exit(1)
	}
}

func (s *SubscriberService) subscribeToUserDeleted() {
	_, err := s.natsConn.Subscribe("users.deleted", func(msg *nats.Msg) {
		slog.Info("user deleted event received")

		// For deletes, we might just get the ID
		var deleteEvent struct {
			Id string `json:"id"`
		}
		if err := json.Unmarshal(msg.Data, &deleteEvent); err != nil {
			slog.Error("failed to unmarshal delete event", "error", err)
			return
		}

		_, err := s.usersClient.Collection("users").Document(deleteEvent.Id).Delete(context.Background())
		if err != nil {
			slog.Error("failed to delete user from Typesense", "error", err)
			return
		}

		slog.Info("deleted user from search index", "id", deleteEvent.Id)
	})

	if err != nil {
		slog.Error("failed to subscribe to users.deleted", "error", err)
		os.Exit(1)
	}
}

func (s *SubscriberService) subscribeToPostUpsert() {
	_, err := s.natsConn.Subscribe("posts.upsert", func(msg *nats.Msg) {
		slog.Info("post upsert event received")

		var post Post
		if err := json.Unmarshal(msg.Data, &post); err != nil {
			slog.Error("failed to unmarshal post", "error", err)
			return
		}

		document := s.postToDocument(post)

		_, err := s.postsClient.Collection("posts").Documents().Upsert(context.Background(), document)
		if err != nil {
			slog.Error("failed to upsert post in Typesense", "error", err)
			return
		}

		slog.Info("upserted post in search index", "id", post.Id)
	})

	if err != nil {
		slog.Error("failed to subscribe to posts.upsert", "error", err)
		os.Exit(1)
	}
}

func (s *SubscriberService) subscribeToPostDeleted() {
	_, err := s.natsConn.Subscribe("posts.deleted", func(msg *nats.Msg) {
		slog.Info("post deleted event received")

		var deleteEvent struct {
			Id string `json:"id"`
		}
		if err := json.Unmarshal(msg.Data, &deleteEvent); err != nil {
			slog.Error("failed to unmarshal delete event", "error", err)
			return
		}

		_, err := s.postsClient.Collection("posts").Document(deleteEvent.Id).Delete(context.Background())
		if err != nil {
			slog.Error("failed to delete post from Typesense", "error", err)
			return
		}

		slog.Info("deleted post from search index", "id", deleteEvent.Id)
	})

	if err != nil {
		slog.Error("failed to subscribe to posts.deleted", "error", err)
		os.Exit(1)
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