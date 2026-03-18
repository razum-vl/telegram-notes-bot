package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

// Note представляет заметку
type Note struct {
	Title     string
	Content   string
	CreatedAt time.Time
}

// Storage хранилище заметок в памяти
type Storage struct {
	mu    sync.RWMutex
	notes map[int64]map[string]Note // userID -> title -> note
}

// NewStorage создает новое хранилище
func NewStorage() *Storage {
	return &Storage{
		notes: make(map[int64]map[string]Note),
	}
}

// Save сохраняет заметку
func (s *Storage) Save(userID int64, title, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Инициализируем хранилище для пользователя если нужно
	if _, exists := s.notes[userID]; !exists {
		s.notes[userID] = make(map[string]Note)
	}

	// Сохраняем заметку
	s.notes[userID][title] = Note{
		Title:     title,
		Content:   content,
		CreatedAt: time.Now(),
	}
}

// Get возвращает заметку
func (s *Storage) Get(userID int64, title string) (Note, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userNotes, exists := s.notes[userID]
	if !exists {
		return Note{}, false
	}

	note, exists := userNotes[title]
	return note, exists
}

// GetAll возвращает все заметки пользователя
func (s *Storage) GetAll(userID int64) []Note {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userNotes, exists := s.notes[userID]
	if !exists {
		return []Note{}
	}

	notes := make([]Note, 0, len(userNotes))
	for _, note := range userNotes {
		notes = append(notes, note)
	}
	return notes
}

// Delete удаляет заметку
func (s *Storage) Delete(userID int64, title string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	userNotes, exists := s.notes[userID]
	if !exists {
		return false
	}

	if _, exists := userNotes[title]; !exists {
		return false
	}

	delete(userNotes, title)
	return true
}

func main() {
	// Загружаем .env файл
	if err := godotenv.Load(); err != nil {
		log.Println("Файл .env не найден, используем переменные окружения")
	}

	// Получаем токен
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN не установлен")
	}

	// Создаем бота
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal(err)
	}

	bot.Debug = true // Включите для отладки
	log.Printf("Бот запущен: %s", bot.Self.UserName)

	// Создаем хранилище
	storage := NewStorage()

	// Настраиваем получение обновлений
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	// Обрабатываем сообщения
	for update := range updates {
		if update.Message == nil {
			continue
		}

		// Каждое сообщение обрабатываем в отдельной горутине
		go handleMessage(bot, storage, update)
	}
}

// handleMessage обрабатывает сообщение
func handleMessage(bot *tgbotapi.BotAPI, storage *Storage, update tgbotapi.Update) {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")

	// Обработка команд
	if update.Message.IsCommand() {
		switch update.Message.Command() {
		case "start":
			msg.Text = "📝 *Бот для заметок*\n\n" +
				"Команды:\n" +
				"/save Заголовок; Текст - сохранить заметку\n" +
				"/get Заголовок - получить заметку\n" +
				"/list - список заметок\n" +
				"/delete Заголовок - удалить заметку"
			msg.ParseMode = "Markdown"

		case "save":
			handleSave(bot, storage, update)

		case "get":
			handleGet(bot, storage, update)

		case "list":
			handleList(bot, storage, update)

		case "delete":
			handleDelete(bot, storage, update)

		default:
			msg.Text = "Неизвестная команда. Используйте /start"
			bot.Send(msg)
		}
	} else {
		// Если не команда - показываем подсказку
		msg.Text = "Используйте команды:\n" +
			"/save Заголовок; Текст - сохранить заметку\n" +
			"/get Заголовок - получить заметку\n" +
			"/list - список заметок\n" +
			"/delete Заголовок - удалить заметку"
		bot.Send(msg)
	}
}

// handleSave сохраняет заметку
func handleSave(bot *tgbotapi.BotAPI, storage *Storage, update tgbotapi.Update) {
	userID := update.Message.From.ID
	args := strings.TrimSpace(update.Message.CommandArguments())

	// Парсим заголовок и текст (формат: "Заголовок; Текст")
	parts := strings.SplitN(args, ";", 2)
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID,
			"❌ Неверный формат. Используйте:\n/save Заголовок; Текст заметки")
		bot.Send(msg)
		return
	}

	title := strings.TrimSpace(parts[0])
	content := strings.TrimSpace(parts[1])

	if title == "" || content == "" {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID,
			"❌ Заголовок и текст не могут быть пустыми")
		bot.Send(msg)
		return
	}

	// Сохраняем заметку
	storage.Save(userID, title, content)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID,
		fmt.Sprintf("✅ Заметка *%s* сохранена!", title))
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

// handleGet получает заметку
func handleGet(bot *tgbotapi.BotAPI, storage *Storage, update tgbotapi.Update) {
	userID := update.Message.From.ID
	title := strings.TrimSpace(update.Message.CommandArguments())

	if title == "" {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID,
			"❌ Укажите заголовок: /get Заголовок")
		bot.Send(msg)
		return
	}

	note, exists := storage.Get(userID, title)
	if !exists {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID,
			fmt.Sprintf("❌ Заметка '%s' не найдена", title))
		bot.Send(msg)
		return
	}

	msg := tgbotapi.NewMessage(update.Message.Chat.ID,
		fmt.Sprintf("📝 *%s*\n\n%s\n\n_Создано: %s_",
			note.Title,
			note.Content,
			note.CreatedAt.Format("02.01.2006 15:04")))
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

// handleList показывает список заметок
func handleList(bot *tgbotapi.BotAPI, storage *Storage, update tgbotapi.Update) {
	userID := update.Message.From.ID
	notes := storage.GetAll(userID)

	if len(notes) == 0 {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID,
			"📭 У вас пока нет заметок. Создайте первую:\n/save Заголовок; Текст")
		bot.Send(msg)
		return
	}

	// Формируем список
	var sb strings.Builder
	sb.WriteString("📋 *Ваши заметки:*\n\n")

	for i, note := range notes {
		sb.WriteString(fmt.Sprintf("%d. *%s* — _%s_\n",
			i+1,
			note.Title,
			note.CreatedAt.Format("02.01.2006")))
	}

	sb.WriteString("\n_Для просмотра: /get Заголовок_")

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, sb.String())
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

// handleDelete удаляет заметку
func handleDelete(bot *tgbotapi.BotAPI, storage *Storage, update tgbotapi.Update) {
	userID := update.Message.From.ID
	title := strings.TrimSpace(update.Message.CommandArguments())

	if title == "" {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID,
			"❌ Укажите заголовок: /delete Заголовок")
		bot.Send(msg)
		return
	}

	if storage.Delete(userID, title) {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID,
			fmt.Sprintf("✅ Заметка *%s* удалена", title))
		msg.ParseMode = "Markdown"
		bot.Send(msg)
	} else {
		msg := tgbotapi.NewMessage(update.Message.Chat.ID,
			fmt.Sprintf("❌ Заметка '%s' не найдена", title))
		bot.Send(msg)
	}
}
