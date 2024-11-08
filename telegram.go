package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/term"
)

type TelegramClient struct {
	client *telegram.Client
	auth   Authenticator
	cfg    Telegram
}

func NewTelegramClient(cfg Telegram) (*TelegramClient, error) {
	if cfg.AppHash == "" || cfg.AppId == 0 || cfg.PhoneNumber == "" || cfg.TrojanContactName == "" {
		return nil, errors.New("invalid config")
	}

	client := telegram.NewClient(cfg.AppId, cfg.AppHash, telegram.Options{})
	auth := Authenticator{PhoneNumber: cfg.PhoneNumber}

	return &TelegramClient{client: client, auth: auth, cfg: cfg}, nil
}

func (c *TelegramClient) Run(ctx context.Context) error {
	return c.client.Run(ctx, c.runFunc())
}

func (c *TelegramClient) runFunc() func(ctx context.Context) error {
	return func(ctx context.Context) error {
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

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
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

				if err := c.sendMessage(ctx, api, trojanUser, token); err != nil {
					slog.Error("failed to send message", "error", err)
					continue
				}

				msg, err := c.retrieveLastMessage(ctx, api, trojanUser)
				if err != nil {
					slog.Error("failed to retrieve last message", "error", err)
					continue
				}

				if err := c.clickButton(ctx, msg, api, trojanUser, 2, 0); err != nil {
					slog.Error("failed to click on button", "error", err)
					continue
				}
			}
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

func (c *TelegramClient) sendMessage(ctx context.Context, api *tg.Client, user *tg.User, token string) error {
	req := &tg.MessagesSendMessageRequest{
		Message:  token,
		Peer:     user.AsInputPeer(),
		RandomID: rand.Int63(), // Random ID to avoid duplicate messages
	}

	if _, err := api.MessagesSendMessage(ctx, req); err != nil {
		return err
	}

	time.Sleep(time.Second)
	return nil
}

func (c *TelegramClient) retrieveLastMessage(ctx context.Context, api *tg.Client, user *tg.User) (*tg.Message, error) {
	updates, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  user.AsInputPeer(),
		Limit: 10,
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

func (c *TelegramClient) clickButton(ctx context.Context, msg *tg.Message, api *tg.Client, user *tg.User, row, column int) error {
	if msg.ReplyMarkup == nil {
		return fmt.Errorf("no reply markup")
	}

	markup, ok := msg.ReplyMarkup.(*tg.ReplyInlineMarkup)
	if !ok {
		return fmt.Errorf("unexpected reply markup type: %T", msg.ReplyMarkup)
	}

	if row >= len(markup.Rows) {
		return fmt.Errorf("row %d out of bounds", row)
	}

	rowButtons := markup.Rows[row].Buttons
	if column >= len(rowButtons) {
		return fmt.Errorf("column %d out of bounds", column)
	}

	button := rowButtons[column]
	buttonCallback, ok := button.(*tg.KeyboardButtonCallback)
	if !ok {
		return fmt.Errorf("unexpected button data type: %T", button)
	}

	if !strings.Contains(buttonCallback.Text, "SOL") {
		return fmt.Errorf("unexpected button data: %s", buttonCallback.Text)
	}

	_, err := api.MessagesGetBotCallbackAnswer(ctx, &tg.MessagesGetBotCallbackAnswerRequest{
		Peer:  user.AsInputPeer(),
		MsgID: msg.ID,
		Data:  buttonCallback.Data,
	})
	if err != nil {
		return err
	}

	slog.Info("buy order placed successfully", "button", buttonCallback.Text)
	time.Sleep(time.Second)

	replyMsg, err := c.retrieveLastMessage(ctx, api, user)
	if err != nil {
		return err
	}

	slog.Info("reply message", "message", replyMsg.Message)
	return nil
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
