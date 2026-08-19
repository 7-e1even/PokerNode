package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"pokernode/internal/app"
	"pokernode/internal/auth"
	"pokernode/internal/secure"
	"pokernode/internal/store"
	"pokernode/internal/wechat"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	address := envOr("POKERNODE_ADDR", ":8080")
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	cipher, err := secure.NewCipher(os.Getenv("POKERNODE_ENCRYPTION_KEY"))
	if err != nil {
		logger.Error("invalid encryption configuration", "error", err)
		os.Exit(1)
	}
	sessions, err := auth.NewSessions(os.Getenv("POKERNODE_SESSION_SECRET"), 7*24*time.Hour)
	if err != nil {
		logger.Error("invalid session configuration", "error", err)
		os.Exit(1)
	}
	database, err := store.Open(databaseURL)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	var serverOptions []app.ServerOption
	if origins := os.Getenv("POKERNODE_TRUSTED_ORIGINS"); origins != "" {
		serverOptions = append(serverOptions, app.WithWebSocketOrigins(strings.Split(origins, ",")...))
	}
	wechatAppID := os.Getenv("WECHAT_APP_ID")
	wechatAppSecret := os.Getenv("WECHAT_APP_SECRET")
	wechatRedirectURI := os.Getenv("WECHAT_REDIRECT_URI")
	if wechatAppID != "" && wechatAppSecret != "" && wechatRedirectURI != "" {
		serverOptions = append(serverOptions, app.WithWeChat(wechatRedirectURI, wechat.NewClient(wechatAppID, wechatAppSecret, nil)))
	} else if wechatAppID != "" || wechatAppSecret != "" || wechatRedirectURI != "" {
		logger.Warn("wechat login is disabled because its configuration is incomplete")
	}

	server := &http.Server{
		Addr:              address,
		Handler:           app.NewServer(database, cipher, sessions, logger, serverOptions...).Handler("./web/dist"),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		logger.Info("PokerNode listening", "address", address)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("serve", "error", err)
			os.Exit(1)
		}
	}()

	stop, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	<-stop.Done()
	ctx, shutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdown()
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("shutdown", "error", err)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
