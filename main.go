package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"google.golang.org/api/option"
)

var (
	channelID          = "UC4oN_lEAD2Adg2yeU_psq5w"
	searchQuery        = "змей"
	TELEGRAM_BOT_TOKEN = os.Getenv("TELEGRAM_BOT_TOKEN")
	YOUTUBE_API_KEY    = os.Getenv("YOUTUBE_API_KEY")
)

type UserLastCommand struct {
	LastUsage time.Time
	Count     int
}

var (
	userCommands = make(map[int64]*UserLastCommand)
	commandMutex sync.RWMutex

	rateLimit = 3
	cooldown  = time.Minute

	authorizedUsers = make(map[int64]bool)
	authMutex       sync.RWMutex
)

func init() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Ошибка загрузки .env файла")
	}
}

func isCommandAllowed(userID int64) bool {
	commandMutex.Lock()
	defer commandMutex.Unlock()

	now := time.Now()
	if cmd, exists := userCommands[userID]; exists {
		if now.Sub(cmd.LastUsage) > cooldown {
			cmd.Count = 1
			cmd.LastUsage = now
			return true
		}

		if cmd.Count >= rateLimit {
			return false
		}

		cmd.Count++
		cmd.LastUsage = now
		return true
	}

	userCommands[userID] = &UserLastCommand{
		LastUsage: now,
		Count:     1,
	}
	return true
}

func isUserAuthorized(userID int64) bool {
	authMutex.RLock()
	defer authMutex.RUnlock()
	return authorizedUsers[userID]
}

func main() {
	ctx := context.Background()
	youtubeService, err := youtube.NewService(ctx, option.WithAPIKey(YOUTUBE_API_KEY))
	if err != nil {
		log.Fatalf("Ошибка при создании YouTube клиента: %v", err)
	}

	// Инициализация Telegram бота
	bot, err := tgbotapi.NewBotAPI(TELEGRAM_BOT_TOKEN)
	if err != nil {
		log.Fatalf("Ошибка при создании Telegram бота: %v", err)
	}

	// Настройка обновлений
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 180
	updateConfig.Offset = -1

	updates := bot.GetUpdatesChan(updateConfig)

	// Обработка сообщений
	for update := range updates {
		if update.Message == nil {
			continue
		}

		userID := update.Message.From.ID

		// Обработка команды /start
		if update.Message.Command() == "start" && update.Message.Chat.Type == "private" {
			authMutex.Lock()
			authorizedUsers[userID] = true
			authMutex.Unlock()

			msg := tgbotapi.NewMessage(userID, "Вы успешно авторизованы! Теперь вы можете использовать команду /snake в любом чате.")
			bot.Send(msg)
			continue
		}

		// Проверка авторизации для команды snake
		if update.Message.Command() == "snake" {
			if !isUserAuthorized(userID) {
				if !isCommandAllowed(userID) {
					message := "Пожалуйста, подождите 3 минуты перед следующим использованием команды"

					// Пробуем отправить в личку
					privateMsg := tgbotapi.NewMessage(userID, message)
					if _, err := bot.Send(privateMsg); err != nil {
						if _, err := bot.Send(privateMsg); err != nil {
							log.Printf("Ошибка при отправке сообщения: %v", err)
						}
					}
					continue
				}

				call := youtubeService.Search.List([]string{"id", "snippet"}).
					ChannelId(channelID).
					Q(searchQuery).
					Type("video").
					MaxResults(1).
					Order("date")

				response, err := call.Do()
				if err != nil {
					log.Printf("Ошибка при поиске видео: %v", err)
					continue
				}

				if len(response.Items) > 0 {
					item := response.Items[0]
					videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", item.Id.VideoId)
					message := fmt.Sprintf("Последнее видео со змеем:\n%s\nНазвание: %s",
						videoURL,
						item.Snippet.Title,
					)

					msg := tgbotapi.NewMessage(update.Message.Chat.ID, message)
					if _, err := bot.Send(msg); err != nil {
						log.Printf("Ошибка при отправке сообщения: %v", err)
					}
				} else {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Видео со змеем не найдены")
					bot.Send(msg)
				}
			}
		}
	}
}
