package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/patrulek/trojanbotproxy/config"
	"golang.org/x/term"
)

type DataSource interface {
	Retrieve(ctx context.Context) ([]string, error)
}

type TelegramClient struct {
	client *telegram.Client
	auth   Authenticator
	cfg    config.Telegram

	ds           DataSource
	boughtTokens map[string]struct{}

	doneC chan struct{}
}

func NewTelegramClient(cfg config.Telegram, ds DataSource) (*TelegramClient, error) {
	if cfg.AppHash == "" || cfg.AppId == 0 || cfg.PhoneNumber == "" || cfg.TrojanContactName == "" {
		return nil, errors.New("invalid config")
	}

	client := telegram.NewClient(cfg.AppId, cfg.AppHash, telegram.Options{})
	auth := Authenticator{PhoneNumber: cfg.PhoneNumber}

	return &TelegramClient{
		client:       client,
		auth:         auth,
		cfg:          cfg,
		ds:           ds,
		boughtTokens: make(map[string]struct{}),
		doneC:        make(chan struct{}),
	}, nil
}

func (c *TelegramClient) Start(ctx context.Context) error {
	slog.Info("starting telegram client")

	return c.client.Run(ctx, c.runFunc())
}

func (c *TelegramClient) runFunc() func(ctx context.Context) error {
	return func(ctx context.Context) error {
		defer close(c.doneC)

		if err := c.authenticate(ctx); err != nil {
			return err
		}

		api := c.client.API()

		channel, err := api.ContactsResolveUsername(ctx, c.cfg.TrojanContactName)
		if err != nil {
			return err
		}

		if channel == nil {
			return errors.New("contact not found")
		}

		if len(channel.Users) == 0 {
			return errors.New("no users in channel")
		}

		trojanUser, _ := channel.Users[0].AsNotEmpty()

		if c.ds == nil {
			return c.runCmdFunc(ctx, api, trojanUser)
		}

		return c.runDataSourceFunc(ctx, api, trojanUser)
	}
}

func (c *TelegramClient) runCmdFunc(ctx context.Context, api *tg.Client, user *tg.User) error {
	interruptC := make(chan os.Signal, 1)
	signal.Notify(interruptC, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-interruptC:
			return nil
		default:
			token, err := c.readTokenAddress()
			if err != nil {
				slog.Error("failed to read token address", "error", err)
				continue
			}

			if token == "quit" || token == "exit" {
				slog.Info("exiting...")
				return nil
			}

			if _, ok := c.boughtTokens[token]; ok {
				slog.Info("token already bought", "token", token)
				continue
			}

			fctx, fcancel := context.WithTimeout(ctx, 75*time.Second) // 75s to paste token and wait for tx confirmation
			err = c.buyToken(fctx, api, user, token)
			fcancel()

			if err != nil {
				slog.Error("failed to buy token", "error", err)
				continue
			}

			c.boughtTokens[token] = struct{}{}
			slog.Info("token bought", "token", token)
		}
	}
}

func (c *TelegramClient) runDataSourceFunc(ctx context.Context, api *tg.Client, user *tg.User) error {
	interruptC := make(chan os.Signal, 1)
	signal.Notify(interruptC, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-interruptC:
			slog.Info("received interrupt signal")
			return nil
		case <-ticker.C:
			tokens, err := c.ds.Retrieve(ctx)
			if err != nil {
				slog.Error("failed to retrieve tokens", "error", err)
				continue
			}

			if len(tokens) == 0 {
				continue // No data
			}

			for _, token := range tokens {
				if _, ok := c.boughtTokens[token]; ok {
					slog.Info("token already bought", "token", token)
					continue
				}

				fctx, fcancel := context.WithTimeout(ctx, 75*time.Second) // 75s to paste token and wait for tx confirmation
				err = c.buyToken(fctx, api, user, token)
				fcancel()

				if err != nil {
					slog.Error("failed to buy token", "error", err)
					continue
				}

				c.boughtTokens[token] = struct{}{}
				slog.Info("token bought", "token", token)
			}
		}
	}
}

func (c *TelegramClient) buyToken(ctx context.Context, api *tg.Client, user *tg.User, token string) error {
	if _, err := c.sendMessage(ctx, api, user, token); err != nil {
		return err
	}

	retries := 5
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if retries == 0 {
				return errors.New("retries exceeded")
			}

			msg, err := c.retrieveLastMessage(ctx, api, user)
			if err != nil {
				slog.Error("failed to retrieve last message", "error", err)
				retries--
				continue
			}

			retries = 5

			if strings.EqualFold(msg.Message, token) {
				// token message, wait for channel updates
				continue
			}

			if strings.Contains(msg.Message, "Token not found") {
				// token not found
				return fmt.Errorf("token not found: %s", msg.Message)
			}

			if strings.Contains(msg.Message, "Transaction sent") {
				// need to wait for confirmation
				continue
			}

			if strings.Contains(msg.Message, "Insufficient balance") {
				// not bought due to insufficient balance
				return fmt.Errorf("insufficient balance: %s", msg.Message)
			}

			if strings.Contains(msg.Message, "tx might have timed out") || strings.Contains(msg.Message, "confirm before retrying") {
				// probably not bought due to tx might have timed out
				return fmt.Errorf("tx might have timed out: %s", msg.Message)
			}

			if !strings.Contains(msg.Message, "Buy Success!") {
				// dont know if bought or not, continue
				continue
			}

			// buy success
			return nil
		}
	}
}

func (c *TelegramClient) authenticate(ctx context.Context) error {
	authFlow := auth.NewFlow(c.auth, auth.SendCodeOptions{})
	return c.client.Auth().IfNecessary(ctx, authFlow)
}

func (c *TelegramClient) readTokenAddress() (string, error) {
	fmt.Print("Enter token address: ")

	token, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(token), nil
}

func (c *TelegramClient) sendMessage(ctx context.Context, api *tg.Client, user *tg.User, token string) (tg.UpdatesClass, error) {
	req := &tg.MessagesSendMessageRequest{
		Message:  token,
		Peer:     user.AsInputPeer(),
		RandomID: rand.Int63(), // Random ID to avoid duplicate messages
	}

	return api.MessagesSendMessage(ctx, req)
}

func (c *TelegramClient) retrieveLastMessage(ctx context.Context, api *tg.Client, user *tg.User) (*tg.Message, error) {
	updates, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  user.AsInputPeer(),
		Limit: 3, // Get last 3 messages
	})
	if err != nil {
		return nil, err
	}

	var messages []tg.MessageClass

	switch v := updates.(type) {
	case *tg.MessagesMessages: // messages.messages#8c718e87
		return nil, fmt.Errorf("unexpected messages type: %T", v)
	case *tg.MessagesMessagesSlice: // messages.messagesSlice#3a54685e
		messages = v.Messages
	case *tg.MessagesChannelMessages: // messages.channelMessages#c776ba4e
		return nil, fmt.Errorf("unexpected messages type: %T", v)
	case *tg.MessagesMessagesNotModified: // messages.messagesNotModified#74535f21
		return nil, fmt.Errorf("unexpected messages type: %T", v)
	default:
		panic(fmt.Sprintf("unsupported messages type: %T", v))
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages found")
	}

	lastMessage, ok := messages[0].(*tg.Message)
	if !ok {
		return nil, fmt.Errorf("unexpected message type: %T", messages[0])
	}

	return lastMessage, nil
}

// Authenticator implements auth.UserAuthenticator prompting the console for input.
type Authenticator struct {
	PhoneNumber string // optional, will be prompted if empty
}

func (Authenticator) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("signing up not implemented in console")
}

func (Authenticator) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return &auth.SignUpRequired{TermsOfService: tos}
}

func (Authenticator) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	fmt.Print("Enter code: ")

	code, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(code), nil
}

func (a Authenticator) Phone(_ context.Context) (string, error) {
	if a.PhoneNumber != "" {
		return a.PhoneNumber, nil
	}

	fmt.Print("Enter phone in international format (e.g. +1234567890): ")

	phone, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(phone), nil
}

func (Authenticator) Password(_ context.Context) (string, error) {
	fmt.Print("Enter 2FA password: ")

	bytePwd, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(bytePwd)), nil
}
