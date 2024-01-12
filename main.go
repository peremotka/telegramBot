package main

import (
	"encoding/json"
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/gorilla/mux"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

var (
	telegramToken  = "6783969097:AAGc8Qvnv1Y_S_clJqU413Cd8d6E08y3_V0"
	bot            *tgbotapi.BotAPI
	globalSessions Sessions
	server         = "https://007c-139-28-177-132.ngrok-free.app"
)

// Структура для хранения данных о пользователе
type User struct {
	Git_id string `json:"id"`
	Role   string `json:"role"`
	Name   string `json:"username"`
	Group  string `json:"group"`
	Tg_id  string `json:"tg_id"`
}

// Тип данных для хранения открытых сессий
type Sessions map[int64]User

func main() {

	go startServer()

	var err error
	bot, err = tgbotapi.NewBotAPI(telegramToken)

	if err != nil {
		log.Fatal(err)
	}

	// Настраиваем обработчик сообщений. Получение обновлений от Telegram API
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60

	updates, err := bot.GetUpdatesChan(updateConfig) /* updates - переменная для получения обновлений
	с платформы (например, новые сообщения) */

	for update := range updates {
		if update.Message == nil { // игнорируем обновления, не являющиеся сообщениями
			if update.CallbackQuery != nil {
				handleCallbackQuery(update) // обрабатываем callback-запросы
			}

			continue
		}

		// Получаем chat_id пользователя
		chatID := update.Message.Chat.ID
		authorized := IsAuthorized(chatID)

		messageText := update.Message.Text

		if messageText == "/login" {
			if !authorized { // если нету chat_id в открытых сессиях
				// Отправка запроса модулю авторизации для получения ссылки авторизации
				authURL, Aerr := GetAuthorizationURL(chatID)

				if Aerr != nil {
					log.Println(Aerr)
				}

				// Отправка сообщения пользователю с ссылкой авторизации
				msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Пожалуйста, [войдите в систему](%s).", authURL))
				msg.ParseMode = "Markdown"
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
			} else {
				msg := tgbotapi.NewMessage(chatID, "Вы уже вошли в систему, продолжайте использовать бота.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
			}

		} else if messageText == "/start" {
			welcomeMessage := "Приветствую!\nЭто телеграмм бот для расписания учебных занятий. Чтобы использовать бот, вам нужно авторизоваться.\n\nДля более детальных сведений введите команду /help."
			msg := tgbotapi.NewMessage(chatID, welcomeMessage)
			_, err := bot.Send(msg)
			if err != nil {
				log.Println(err)
			}

		} else if messageText == "/help" {
			helpMess := "Для начала, используйте команду /login и перейдите по появившейся ссылке. Войдите в свой аккаунт на GitHub. Если авторизация прошла успешно, вы получите соответствующее сообщение.\n\nЕсли вы не войдете в свой аккаунт на GitHub, вы не сможете использовать нашего Telegram-бота для получения расписания учебных занятий.\n\nЧтобы выйти, используйте команду /logout."
			msg := tgbotapi.NewMessage(chatID, helpMess)

			_, err := bot.Send(msg)
			if err != nil {
				log.Println(err)
			}

		} else if messageText == "/logout" {
			if authorized {
				DeleteSession(chatID)
				msg := tgbotapi.NewMessage(chatID, "Вы вышли из системы.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
			} else {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не авторизовались, пожалуйста, авторизуйтесь /login.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
			}

		} else if messageText == "/whoami" {
			user, err := GetUserByChatID(chatID)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не авторизовались, пожалуйста, авторизуйтесь /login.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "student" {
				if user.Name == "" {
					Message := "Пришлите мне ваше полное ФИО.\nПример: Иванов Александр Александрович"
					msg := tgbotapi.NewMessage(chatID, Message)
					_, err := bot.Send(msg)
					if err != nil {
						log.Println(err)
					}
					// Обновляем переменную nameMessageText новым значением приема сообщений от пользователя
					nameMessage := <-updates
					new_name := nameMessage.Message.Text

					UpdateUserName(chatID, new_name)

					user.Name = new_name
				}
				if user.Group == "" {
					Message := tgbotapi.NewMessage(chatID, fmt.Sprintf("%s, введите номер вашей группы.\nПример: 232(1)", user.Name))
					_, err2 := bot.Send(Message)
					if err2 != nil {
						log.Println(err2)
					}

					groupMessage := <-updates
					new_group := groupMessage.Message.Text

					UpdateUserGroup(chatID, new_group)

					user.Group = new_group
				}
			} else if user.Role == "teacher" {
				if user.Name == "" {
					Message := "Пришлите мне ваше вашу фамилию и инциалы.\nПример: Иванов А.А."
					msg := tgbotapi.NewMessage(chatID, Message)
					_, err := bot.Send(msg)
					if err != nil {
						log.Println(err)
					}

					nameMessage := <-updates
					new_name := nameMessage.Message.Text

					UpdateUserName(chatID, new_name)

					user.Name = new_name

				}
			} else if user.Role == "admin" {
				if user.Name == "" || user.Group == "" {
					Message := "Вы можете использовать /change_name или /change_group, чтобы добавить/изменить имя и группу соответственно."
					msg := tgbotapi.NewMessage(chatID, Message)
					_, err := bot.Send(msg)
					if err != nil {
						log.Println(err)
					}
				}
			}

			// вывод сообщения о том, who is user?
			if user.Name != "" && user.Group != "" {
				Message := "Вы авторизованы как " + user.Name + " из группы " + user.Group
				msg := tgbotapi.NewMessage(chatID, Message)
				_, err1 := bot.Send(msg)
				if err1 != nil {
					log.Println(err1)
				}
			} else if user.Name != "" && user.Group == "" {
				Message := "Вы авторизованы как " + user.Name
				msg := tgbotapi.NewMessage(chatID, Message)
				_, err1 := bot.Send(msg)
				if err1 != nil {
					log.Println(err1)
				}
			}

		} else if messageText == "/actions" {
			user, err := GetUserByChatID(chatID)

			if err != nil {
				fmt.Printf("Ошибка при получении пользователя: %v\n", err)

				msg := tgbotapi.NewMessage(chatID, "Произошла ошибка. Если еще не авторизовались, пожалуйста, авторизуйтесь /login. В противном случае, попробуйте позже.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println("Ошибка при отправке сообщения: ", err)
				}
				continue
			}

			named := IsNamed(chatID)

			if user.Role == "student" && named == false {
				// Пользователь авторизован, но не назвался
				msg := tgbotapi.NewMessage(chatID, "Вы еще не назвались, пожалуйста, назовитесь /whoami.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "teacher" && user.Name == "" {
				// Пользователь авторизован, но не назвался
				msg := tgbotapi.NewMessage(chatID, "Вы еще не назвались, пожалуйста, назовитесь /whoami.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "admin" {
				var replyMarkup tgbotapi.InlineKeyboardMarkup

				btn := tgbotapi.NewInlineKeyboardButtonData("Начать сеанс администрирования", "admin")

				replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
					tgbotapi.NewInlineKeyboardRow(btn),
				)

				msg := tgbotapi.NewMessage(chatID, "Выберите действие:")
				msg.ReplyMarkup = replyMarkup
				_, err = bot.Send(msg)
				if err != nil {
					log.Println(err)
				}

			} else {
				keyboardMarkup := getMainKeyboard(user.Role)
				msg := tgbotapi.NewMessage(chatID, "Выберите действие:")
				msg.ReplyMarkup = keyboardMarkup
				_, err = bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
			}

		} else if messageText == "/change_name" {
			user, err := GetUserByChatID(chatID)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не авторизовались, пожалуйста, авторизуйтесь /login.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			named := IsNamed(chatID)

			if user.Role == "student" && named == false {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не назвались, пожалуйста, назовитесь /whoami.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "teacher" && user.Name == "" {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не назвались, пожалуйста, назовитесь /whoami.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "student" {
				Message := "Пожалуйста, пришлите мне свои обновленные данные, включая ваше полное ФИО.\nПример: Иванов Александр Александрович"
				msg := tgbotapi.NewMessage(chatID, Message)
				_, err := bot.Send(msg)
				if err != nil {
					fmt.Println(err)
				}

				nameMessage := <-updates
				new_name := nameMessage.Message.Text

				UpdateUserName(chatID, new_name)

				user.Name = new_name

				mssg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Вы изменили имя на %s", new_name))
				_, err1 := bot.Send(mssg)
				if err1 != nil {
					log.Println(err1)
				}

			} else if user.Role == "teacher" {
				Message := "Пожалуйста, пришлите мне свои обновленные данные, включая вашу фамилию и инициалы.\nПример: Иванов А.А."
				msg := tgbotapi.NewMessage(chatID, Message)
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}

				nameMessage := <-updates
				new_name := nameMessage.Message.Text

				UpdateUserName(chatID, new_name)

				user.Name = new_name

				mssg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Вы изменили имя на %s", new_name))
				_, err1 := bot.Send(mssg)
				if err1 != nil {
					log.Println(err1)
				}
			} else if user.Role == "admin" {
				Message := "Пожалуйста, пришлите мне вашу фамилию и инициалы, если вы учитель (пример: Иванов А.А.).\nЕсли вы студент, пришлите мне ваше полное ФИО (пример: Иванов Александр Александрович)."
				msg := tgbotapi.NewMessage(chatID, Message)
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				nameMessage := <-updates
				new_name := nameMessage.Message.Text

				UpdateUserName(chatID, new_name)

				user.Name = new_name

				mssg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Вы изменили имя на %s", new_name))
				_, err1 := bot.Send(mssg)
				if err1 != nil {
					log.Println(err1)
				}
			}

		} else if messageText == "/change_group" {
			user, err := GetUserByChatID(chatID)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не авторизовались, пожалуйста, авторизуйтесь /login.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			named := IsNamed(chatID)

			if user.Role == "teacher" {
				msg := tgbotapi.NewMessage(chatID, "Эта функция доступна только студентам.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "student" && named == false {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не назвались, пожалуйста, назовитесь /whoami.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "student" && named == true {
				Message := "Пожалуйста, пришлите мне свои обновленные данные - номер группы и подгруппы.\nПример: 232(1)"
				msg := tgbotapi.NewMessage(chatID, Message)
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}

				groupMessage := <-updates
				new_group := groupMessage.Message.Text

				UpdateUserGroup(chatID, new_group)

				user.Group = new_group

				mssg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Вы изменили группу на %s", new_group))
				_, err1 := bot.Send(mssg)
				if err1 != nil {
					log.Println(err1)
				}
			}

			if user.Role == "admin" {
				Message := "Пожалуйста, пришлите мне номер своей группы и подгруппы.\nПример: 232(1)"
				msg := tgbotapi.NewMessage(chatID, Message)
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}

				groupMessage := <-updates
				new_group := groupMessage.Message.Text

				UpdateUserGroup(chatID, new_group)

				user.Group = new_group

				mssg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ваша группа %s", new_group))
				_, err1 := bot.Send(mssg)
				if err1 != nil {
					log.Println(err1)
				}
			}

		} else if messageText == "/wheres_teacher" {
			user, err := GetUserByChatID(chatID)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не авторизовались, пожалуйста, авторизуйтесь /login.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			named := IsNamed(chatID)

			if user.Role == "student" && named == false {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не назвались, пожалуйста, назовитесь /whoami.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "teacher" || user.Role == "admin" {
				msg := tgbotapi.NewMessage(chatID, "Эта функция доступна только пользователям с ролью 'студент'.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			msg0 := tgbotapi.NewMessage(chatID, "Какая у вас сейчас по счету пара?")
			_, err0 := bot.Send(msg0)
			if err0 != nil {
				log.Println(err0)
			}

			lessonNumberFromUser := <-updates
			lessonNumber := lessonNumberFromUser.Message.Text

			msg := tgbotapi.NewMessage(chatID, "Какого преподавателя вы ищете? Введите фамилию и инициалы.\nПример: Иванов И.А.")
			_, err1 := bot.Send(msg)
			if err1 != nil {
				log.Println(err1)
			}

			teacherNameFromUser := <-updates
			teacherName := teacherNameFromUser.Message.Text

			output, err2 := whereIsTeacher(teacherName, lessonNumber)
			if err2 != nil {
				fmt.Println(err2)
			}

			msg1 := tgbotapi.NewMessage(chatID, output)
			_, err3 := bot.Send(msg1)
			if err3 != nil {
				log.Println(err3)
			}

		} else if messageText == "/wheres_group" {
			user, err := GetUserByChatID(chatID)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не авторизовались, пожалуйста, авторизуйтесь /login.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "teacher" && user.Name == "" {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не назвались, пожалуйста, назовитесь /whoami.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "student" || user.Role == "admin" {
				msg := tgbotapi.NewMessage(chatID, "Эта функция доступна только пользователям с ролью 'учитель'.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			msg := tgbotapi.NewMessage(chatID, "Какую группу вы ищите? Пришлите мне номер группы и подгруппы, например: 232(1).")
			_, err0 := bot.Send(msg)
			if err0 != nil {
				log.Println(err0)
			}

			GroupFromUser := <-updates
			NewGroup := GroupFromUser.Message.Text

			msg1 := tgbotapi.NewMessage(chatID, "Укажите номер текущей пары\nПросто введите номер (пример: 1).")
			_, err1 := bot.Send(msg1)
			if err1 != nil {
				log.Println(err1)
			}

			NumberFromUser := <-updates
			numberLesson := NumberFromUser.Message.Text

			output, err2 := whereIsGroup(numberLesson, NewGroup)
			if err2 != nil {
				fmt.Println(err2)
			}

			msg2 := tgbotapi.NewMessage(chatID, output)
			_, err3 := bot.Send(msg2)
			if err3 != nil {
				log.Println(err3)
			}
		} else if messageText == "/leave_comment" {
			user, err := GetUserByChatID(chatID)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не авторизовались, пожалуйста, авторизуйтесь /login.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Name == "" && user.Role == "teacher" {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не назвались, пожалуйста, назовитесь /whoami.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "admin" || user.Role == "student" {
				msg := tgbotapi.NewMessage(chatID, "Эта функция доступна только пользователям с ролью 'учитель'.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			jwt, err := getJWTToken(chatID)

			if err != nil {
				fmt.Println(err)
			}

			msg := tgbotapi.NewMessage(chatID, "Для какой группы вы хотите оставить комментарий?\nВведите номер группы и подгруппы, как показано в примере\nПример: 232(1).")
			_, err1 := bot.Send(msg)
			if err1 != nil {
				log.Println(err1)
			}

			Message_group := <-updates
			Group := Message_group.Message.Text

			msg3 := tgbotapi.NewMessage(chatID, "Укажите день недели, где вы хотите оставить комментарий.\nВведите день недели с прописной буквы\nПример: Понедельник.")
			_, err4 := bot.Send(msg3)
			if err4 != nil {
				log.Println(err4)
			}

			Message_day := <-updates
			Day := Message_day.Message.Text

			msg2 := tgbotapi.NewMessage(chatID, "Укажите номер вашей пары\nПросто введите номер (пример: 1).")
			_, err3 := bot.Send(msg2)
			if err3 != nil {
				log.Println(err3)
			}

			Message_lesson := <-updates
			LessonNumber := Message_lesson.Message.Text

			msg1 := tgbotapi.NewMessage(chatID, "Какой комментарий вы хотите оставить?")
			_, err2 := bot.Send(msg1)
			if err2 != nil {
				log.Println(err2)
			}

			Message_comment := <-updates
			Comment := Message_comment.Message.Text

			output, err := leaveComment(jwt, Group, LessonNumber, Comment, Day, user.Name)

			mes := tgbotapi.NewMessage(chatID, output)
			_, erro := bot.Send(mes)
			if erro != nil {
				log.Println(erro)
			}

		} else if messageText == "/send_token" {
			user, err := GetUserByChatID(chatID)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не авторизовались, пожалуйста, авторизуйтесь /login.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "teacher" || user.Role == "student" {
				msg := tgbotapi.NewMessage(chatID, "Эта функция доступна только пользователям с ролью 'админ'.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			jwt, err := getJWTToken(chatID)
			if err != nil {
				fmt.Println(err)
			}

			msg := tgbotapi.NewMessage(chatID, "Введите раннее скопированный вами из браузера токен, чтобы получить ссылку для перехода в панель администратора:")
			_, err2 := bot.Send(msg)
			if err2 != nil {
				log.Println(err2)
			}

			Message_token := <-updates
			Token := Message_token.Message.Text

			adminURL, err3 := sendToken(jwt, Token)
			if err3 != nil {
				log.Println(err2)
			}

			msg0 := tgbotapi.NewMessage(chatID, fmt.Sprintf("Нажмите на [ссылку для перехода в панель администратора](%s).", adminURL))
			msg0.ParseMode = "Markdown"
			_, err4 := bot.Send(msg0)
			if err4 != nil {
				log.Println("Ошибка", err4)
			}

		} else if messageText == "/where_next_lesson" {
			user, err := GetUserByChatID(chatID)
			if err != nil {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не авторизовались, пожалуйста, авторизуйтесь /login.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			named := IsNamed(chatID)

			if user.Role == "student" && named == false {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не назвались, пожалуйста, назовитесь /whoami.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "teacher" && user.Name == "" {
				msg := tgbotapi.NewMessage(chatID, "Вы еще не назвались, пожалуйста, назовитесь /whoami.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "admin" {
				msg := tgbotapi.NewMessage(chatID, "Эта функция доступна только пользователям с ролью 'студент'.")
				_, err := bot.Send(msg)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			if user.Role == "student" {
				msg0 := tgbotapi.NewMessage(chatID, "Какая у вас сейчас по счету пара?")
				_, err0 := bot.Send(msg0)
				if err0 != nil {
					log.Println(err0)
				}

				lessonNumberFromUser := <-updates
				lessonNumber := lessonNumberFromUser.Message.Text

				output, err1 := nextLessonForStudent(lessonNumber, user.Group)
				if err1 != nil {
					fmt.Println(err1)
				}

				msg := tgbotapi.NewMessage(chatID, output)
				_, err2 := bot.Send(msg)
				if err2 != nil {
					log.Println(err2)
				}
			}

			if user.Role == "teacher" {
				msg0 := tgbotapi.NewMessage(chatID, "Какая у вас сейчас по счету пара?")
				_, err0 := bot.Send(msg0)
				if err0 != nil {
					log.Println(err0)
				}

				lessonNumberFromUser := <-updates
				lessonNumber := lessonNumberFromUser.Message.Text

				output, err1 := nextLessonForTeacher(lessonNumber, user.Name)
				if err1 != nil {
					fmt.Println(err1)
				}

				msg := tgbotapi.NewMessage(chatID, output)
				_, err2 := bot.Send(msg)
				if err2 != nil {
					log.Println(err2)
				}
			}

		}
	}
}

func startServer() { //go get -u github.com/gorilla/mux@v1.8.1
	router := mux.NewRouter()

	// Регистрируем маршруты
	router.HandleFunc("/register-confirm", registerConfirm) // обработчик callback авторизации через гитхаб.
	err := http.ListenAndServe(":8081", router)
	if err != nil {
		log.Fatal("Error starting HTTP server:", err) // return
	} // Запуск сервера (порт бота 8081).
}

func IsAuthorized(chatID int64) bool {
	if globalSessions == nil {
		globalSessions = make(Sessions)
	}
	_, ok := globalSessions[chatID]
	return ok
}

func GetAuthorizationURL(chatID int64) (string, error) {
	strChatId := strconv.FormatInt(chatID, 10)
	resp, err := http.Get("http://localhost:8080/auth?chat_id=" + strChatId)
	if err != nil {
		fmt.Println("Ошибка при выполнении GET-запроса:", err)
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Ошибка при чтении ответа:", err)
		return "", err
	}
	return string(body), err
}

func registerConfirm(w http.ResponseWriter, r *http.Request) {
	chatIDParam := r.URL.Query().Get("chat_id") // принимаем параметр chat_id
	if chatIDParam == "" {
		// Если chatID отсутствует, отправляем ошибку
		http.Error(w, "No chat_id provided", http.StatusBadRequest)
		return
	}
	chatID, err := strconv.ParseInt(chatIDParam, 10, 64)
	if err != nil {
		// Если chatID невалиден, отправляем ошибку
		http.Error(w, "Invalid chat_id", http.StatusBadRequest)
		return
	}

	GithubIDParam := r.URL.Query().Get("github_id") // принимаем параметр github_id
	if GithubIDParam == "" {
		// Если github_id отсутствует, отправляем ошибку
		http.Error(w, "No github_id provided", http.StatusBadRequest)
		return
	}
	github_id, err := strconv.ParseInt(GithubIDParam, 10, 64)

	if err != nil {
		// Если github_id невалиден, отправляем ошибку
		http.Error(w, "Invalid chat_id", http.StatusBadRequest)
		return
	}

	user, err := GetUserByGitID(github_id)

	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	SetSession(*user, chatID) // создаем сессию для пользователя по chatID

	msg := tgbotapi.NewMessage(chatID, "Хорошо, вы успешно авторизовались. Теперь вам нужно ввести свои данные - используйте /whoami.\n\nЕсли вы уже вводили свои данные, данная команда просто выведет их на экран. Используйте /actions, чтобы дальше использовать функционал телеграмм-бота (в том числе, если вы хотите изменить свои данные).")
	bot.Send(msg)

}

func SetSession(user User, userID int64) {
	if globalSessions == nil {
		globalSessions = make(Sessions)
	}
	globalSessions[userID] = user
	log.Printf("Пользователь с ID %d добавлен в сессии", userID)
}

func GetUserByGitID(github_id int64) (*User, error) {
	userURL := fmt.Sprintf("http://localhost:8080/get_all_users")

	resp, err := http.Get(userURL)
	if err != nil {
		log.Printf("Ошибка при выполнении GET-запроса к %s: %v\n", userURL, err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Неправильный статус в ответе запроса поиска пользователя: код %d\n", resp.StatusCode)
		return nil, fmt.Errorf("неправильный статус, код: %d", resp.StatusCode)
	}

	var users []User // Используем слайс, так как ожидаем массив пользователей
	err = json.NewDecoder(resp.Body).Decode(&users)
	if err != nil {
		log.Printf("Ошибка при декодировании JSON ответа в пользователей: %v\n", err)
		return nil, err
	}

	if len(users) == 0 {
		log.Printf("Пользователь с github_id %d не найден\n", github_id)
		return nil, fmt.Errorf("пользователь не найден")
	}

	return &users[0], nil // Возвращаем первого пользователя из списка
}

func DeleteSession(chatID int64) {
	if globalSessions != nil {
		delete(globalSessions, chatID)
	}
}

func IsNamed(chatID int64) bool { // назван ли пользователь (студент)
	user, err := GetUserByChatID(chatID)
	if err != nil {
		fmt.Println("Ошибка функции GetUserByChatID() внутри IsNamed():", err)
		return false
	}

	// Проверяем, не равны ли поля user нулевым значениям, чтобы избежать nil pointer dereference
	if user == nil || user.Group == "" || user.Name == "" {
		return false
	}

	return true
}

func GetUserByChatID(chatID int64) (*User, error) {
	if globalSessions == nil {
		globalSessions = make(Sessions)
	}

	user, ok := globalSessions[chatID]
	if !ok {
		return nil, fmt.Errorf("пользователь с chatID %d не найден", chatID)
	}

	return &user, nil
}

func getKeyboardMarkupByRole(role string) tgbotapi.InlineKeyboardMarkup {
	var replyMarkup tgbotapi.InlineKeyboardMarkup
	if role == "student" {
		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Где следующая пара?", "where_next_lesson"),
				tgbotapi.NewInlineKeyboardButtonData("Расписание на [день недели]", "schedule_by_weekday"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Расписание на сегодня", "today_lessons"),
				tgbotapi.NewInlineKeyboardButtonData("Расписание на завтра", "tomorrow_lessons"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Где преподаватель?", "wheres_teacher"),
				tgbotapi.NewInlineKeyboardButtonData("Когда экзамен?", "exs"),
			),
		)
	} else if role == "teacher" {
		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Где следующая пара?", "where_next_lesson"),
				tgbotapi.NewInlineKeyboardButtonData("Расписание на [день недели]", "schedule_by_weekday_for_group"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Расписание на сегодня", "today_lessons_for_group"),
				tgbotapi.NewInlineKeyboardButtonData("Расписание на завтра", "tomorrow_lessons_for_group"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Оставить комментарий", "leave_comment"),
				tgbotapi.NewInlineKeyboardButtonData("Где группа?", "wheres_group"),
			),
		)
	}
	return replyMarkup
}

func handleCallbackQuery(update tgbotapi.Update) { // обрабатывает нажатия на inline-кнопки в сообщениях бота
	callbackData := update.CallbackQuery.Data
	chatID := update.CallbackQuery.Message.Chat.ID

	if callbackData != "" && chatID != 0 {
		processButtonPress(callbackData, chatID)
	}

	// отправляем ответ на callback-запрос
	cb := tgbotapi.NewCallback(update.CallbackQuery.ID, "") // создаем объект, указываем идентификатор запроса (update.CallbackQuery.ID) и пустую строку в качестве ответа.
	bot.AnswerCallbackQuery(cb)                             // отправляем ответ на callback-запрос, чтобы Telegram знал, что запрос успешно обработан.
}

func getJWTToken(chatID int64) (string, error) {
	user, err := GetUserByChatID(chatID)
	if err != nil {
		fmt.Println(err)
	}
	// Выполняем GET-запрос к модулю авторизации
	resp, err := http.Get("http://localhost:8080/find?github_id=" + user.Git_id)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Проверяем успешность запроса (код 200)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("неправильный статус, код: %d", resp.StatusCode)
	}

	// Читаем тело ответа
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Возвращаем токен из ответа
	return string(body), nil
}

func processButtonPress(callbackData string, chatID int64) {
	user, err := GetUserByChatID(chatID)
	if err != nil {
		fmt.Println(err)
	}

	switch callbackData {

	// Команда "Где следующая пара?"
	case "where_next_lesson":
		Message := "Чтобы узнать, где следующая пара, вызовите /where_next_lesson"
		msg := tgbotapi.NewMessage(chatID, Message)
		_, err := bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	// Команда "Расписание на сегодня"
	case "today_lessons":
		SendingShedule(chatID, user.Group, "today", "scheduleFor")

	case "today_lessons_for_group":
		var replyMarkup tgbotapi.InlineKeyboardMarkup

		btn := tgbotapi.NewInlineKeyboardButtonData("231(1)", "231(1)today")
		btn1 := tgbotapi.NewInlineKeyboardButtonData("231(2)", "231(2)today")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("232(1)", "232(1)today")
		btn3 := tgbotapi.NewInlineKeyboardButtonData("232(2)", "232(2)today")
		btn4 := tgbotapi.NewInlineKeyboardButtonData("233(1)", "233(1)today")
		btn5 := tgbotapi.NewInlineKeyboardButtonData("233(2)", "233(2)today")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn),
			tgbotapi.NewInlineKeyboardRow(btn1),
			tgbotapi.NewInlineKeyboardRow(btn2),
			tgbotapi.NewInlineKeyboardRow(btn3),
			tgbotapi.NewInlineKeyboardRow(btn4),
			tgbotapi.NewInlineKeyboardRow(btn5),
		)

		msg := tgbotapi.NewMessage(chatID, "Для какой группы вы хотите узнать расписание?")
		msg.ReplyMarkup = replyMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "231(1)today":
		SendingShedule(chatID, "231(1)", "today", "scheduleFor")

	case "231(2)today":
		SendingShedule(chatID, "231(2)", "today", "scheduleFor")

	case "232(1)today":
		SendingShedule(chatID, "232(1)", "today", "scheduleFor")

	case "232(2)today":
		SendingShedule(chatID, "232(2)", "today", "scheduleFor")

	case "233(1)today":
		SendingShedule(chatID, "233(1)", "today", "scheduleFor")

	case "233(2)today":
		SendingShedule(chatID, "233(2)", "today", "scheduleFor")

	case "tomorrow_lessons":
		SendingShedule(chatID, user.Group, "tomorrow", "scheduleFor")

	case "tomorrow_lessons_for_group":
		var replyMarkup tgbotapi.InlineKeyboardMarkup

		btn := tgbotapi.NewInlineKeyboardButtonData("231(1)", "231(1)tomorrow")
		btn1 := tgbotapi.NewInlineKeyboardButtonData("231(2)", "231(2)tomorrow")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("232(1)", "232(1)tomorrow")
		btn3 := tgbotapi.NewInlineKeyboardButtonData("232(2)", "232(2)tomorrow")
		btn4 := tgbotapi.NewInlineKeyboardButtonData("233(1)", "233(1)tomorrow")
		btn5 := tgbotapi.NewInlineKeyboardButtonData("233(2)", "233(2)tomorrow")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn),
			tgbotapi.NewInlineKeyboardRow(btn1),
			tgbotapi.NewInlineKeyboardRow(btn2),
			tgbotapi.NewInlineKeyboardRow(btn3),
			tgbotapi.NewInlineKeyboardRow(btn4),
			tgbotapi.NewInlineKeyboardRow(btn5),
		)

		msg := tgbotapi.NewMessage(chatID, "Для какой группы вы хотите узнать расписание?")
		msg.ReplyMarkup = replyMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "231(1)tomorrow":
		SendingShedule(chatID, "231(1)", "tomorrow", "scheduleFor")

	case "231(2)tomorrow":
		SendingShedule(chatID, "231(2)", "tomorrow", "scheduleFor")

	case "232(1)tomorrow":
		SendingShedule(chatID, "232(1)", "tomorrow", "scheduleFor")

	case "232(2)tomorrow":
		SendingShedule(chatID, "232(2)", "tomorrow", "scheduleFor")

	case "233(1)tomorrow":
		SendingShedule(chatID, "233(1)", "tomorrow", "scheduleFor")

	case "233(2)tomorrow":
		SendingShedule(chatID, "233(2)", "tomorrow", "scheduleFor")

	case "wheres_teacher":
		Message := "Чтобы узнать, где находится конкретный преподаватель, вызовите /wheres_teacher."
		msg := tgbotapi.NewMessage(chatID, Message)
		_, err := bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "wheres_group":
		Message := "Чтобы узнать, где находится конкретная группа, вызовите /wheres_group."
		msg := tgbotapi.NewMessage(chatID, Message)
		_, err := bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "admin_button":
		msg := tgbotapi.NewMessage(chatID, "Сообщение")
		_, err := bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "education":
		keyboardMarkup := getKeyboardMarkupByRole(user.Role)
		msg := tgbotapi.NewMessage(chatID, "Выберите действие:")
		msg.ReplyMarkup = keyboardMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "about_you":
		var replyMarkup tgbotapi.InlineKeyboardMarkup

		btn := tgbotapi.NewInlineKeyboardButtonData("Изменить имя", "name")
		btn1 := tgbotapi.NewInlineKeyboardButtonData("Изменить номер группы", "group")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn),
			tgbotapi.NewInlineKeyboardRow(btn1),
		)

		msg := tgbotapi.NewMessage(chatID, "Что вы хотите изменить? ")
		msg.ReplyMarkup = replyMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "about_you2":
		var replyMarkup tgbotapi.InlineKeyboardMarkup

		btn := tgbotapi.NewInlineKeyboardButtonData("Изменить имя", "name")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn),
		)

		msg := tgbotapi.NewMessage(chatID, "Что вы хотите изменить? ")
		msg.ReplyMarkup = replyMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "name":
		Message := "Чтобы изменить имя, вызовите /change_name."
		msg := tgbotapi.NewMessage(chatID, Message)
		_, err := bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "group":
		Message := "Чтобы изменить номер группы, вызовите /change_group."
		msg := tgbotapi.NewMessage(chatID, Message)
		_, err := bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "admin":
		jwt, err := getJWTToken(chatID)
		if err != nil {
			fmt.Println(err)
		}
		adminURL, err1 := startNewAdminSession(chatID, jwt)
		if err1 != nil {
			fmt.Println(err1)
		}
		fmt.Println(adminURL)
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Нажмите на [ссылку для перехода в панель администратора](%s).", adminURL))
		msg.ParseMode = "Markdown"
		_, err2 := bot.Send(msg)
		if err2 != nil {
			log.Println(err2)
		}

	case "exs":
		msg := tgbotapi.NewMessage(chatID, "Точные даты экзаменов появятся позже.")
		_, err2 := bot.Send(msg)
		if err2 != nil {
			log.Println(err2)
		}

	case "schedule_by_weekday":
		var replyMarkup tgbotapi.InlineKeyboardMarkup

		btn := tgbotapi.NewInlineKeyboardButtonData("Понедельник", "monday")
		btn1 := tgbotapi.NewInlineKeyboardButtonData("Вторник", "tuesday")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("Среда", "wednesday")
		btn3 := tgbotapi.NewInlineKeyboardButtonData("Четверг", "thursday")
		btn4 := tgbotapi.NewInlineKeyboardButtonData("Пятница", "friday")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn),
			tgbotapi.NewInlineKeyboardRow(btn1),
			tgbotapi.NewInlineKeyboardRow(btn2),
			tgbotapi.NewInlineKeyboardRow(btn3),
			tgbotapi.NewInlineKeyboardRow(btn4),
		)

		msg := tgbotapi.NewMessage(chatID, "Выберите день недели:")
		msg.ReplyMarkup = replyMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "monday":
		SendingShedule(chatID, user.Group, "Понедельник", "scheduleFor")

	case "tuesday":
		SendingShedule(chatID, user.Group, "Вторник", "scheduleFor")

	case "wednesday":
		SendingShedule(chatID, user.Group, "Среда", "scheduleFor")

	case "thursday":
		SendingShedule(chatID, user.Group, "Четверг", "scheduleFor")

	case "friday":
		SendingShedule(chatID, user.Group, "Пятница", "scheduleFor")

	case "schedule_by_weekday_for_group":
		var replyMarkup tgbotapi.InlineKeyboardMarkup

		btn := tgbotapi.NewInlineKeyboardButtonData("231(1)", "231(1)weekday") // done
		btn1 := tgbotapi.NewInlineKeyboardButtonData("231(2)", "231(2)weekday")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("232(1)", "232(1)weekday")
		btn3 := tgbotapi.NewInlineKeyboardButtonData("232(2)", "232(2)weekday")
		btn4 := tgbotapi.NewInlineKeyboardButtonData("233(1)", "233(1)weekday")
		btn5 := tgbotapi.NewInlineKeyboardButtonData("233(2)", "233(2)weekday")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn),
			tgbotapi.NewInlineKeyboardRow(btn1),
			tgbotapi.NewInlineKeyboardRow(btn2),
			tgbotapi.NewInlineKeyboardRow(btn3),
			tgbotapi.NewInlineKeyboardRow(btn4),
			tgbotapi.NewInlineKeyboardRow(btn5),
		)

		msg := tgbotapi.NewMessage(chatID, "Для какой группы вы хотите узнать расписание?")
		msg.ReplyMarkup = replyMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	// Расписание на разные дни недели для 231(1).
	case "231(1)weekday":
		var replyMarkup tgbotapi.InlineKeyboardMarkup

		btn := tgbotapi.NewInlineKeyboardButtonData("Понедельник", "monday2311")
		btn1 := tgbotapi.NewInlineKeyboardButtonData("Вторник", "tuesday2311")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("Среда", "wednesday2311")
		btn3 := tgbotapi.NewInlineKeyboardButtonData("Четверг", "thursday2311")
		btn4 := tgbotapi.NewInlineKeyboardButtonData("Пятница", "friday2311")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn),
			tgbotapi.NewInlineKeyboardRow(btn1),
			tgbotapi.NewInlineKeyboardRow(btn2),
			tgbotapi.NewInlineKeyboardRow(btn3),
			tgbotapi.NewInlineKeyboardRow(btn4),
		)

		msg := tgbotapi.NewMessage(chatID, "Выберите день недели:")
		msg.ReplyMarkup = replyMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "monday2311":
		SendingShedule(chatID, "231(1)", "Понедельник", "scheduleFor")

	case "tuesday2311":
		SendingShedule(chatID, "231(1)", "Вторник", "scheduleFor")

	case "wednesday2311":
		SendingShedule(chatID, "231(1)", "Среда", "scheduleFor")

	case "thursday2311":
		SendingShedule(chatID, "231(1)", "Четверг", "scheduleFor")

	case "friday2311":
		SendingShedule(chatID, "231(1)", "Пятница", "scheduleFor")

	// Расписание на разные дни недели для 231(2).
	case "231(2)weekday":
		var replyMarkup tgbotapi.InlineKeyboardMarkup

		btn := tgbotapi.NewInlineKeyboardButtonData("Понедельник", "monday2312")
		btn1 := tgbotapi.NewInlineKeyboardButtonData("Вторник", "tuesday2312")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("Среда", "wednesday2312")
		btn3 := tgbotapi.NewInlineKeyboardButtonData("Четверг", "thursday2312")
		btn4 := tgbotapi.NewInlineKeyboardButtonData("Пятница", "friday2312")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn),
			tgbotapi.NewInlineKeyboardRow(btn1),
			tgbotapi.NewInlineKeyboardRow(btn2),
			tgbotapi.NewInlineKeyboardRow(btn3),
			tgbotapi.NewInlineKeyboardRow(btn4),
		)

		msg := tgbotapi.NewMessage(chatID, "Выберите день недели:")
		msg.ReplyMarkup = replyMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "monday2312":
		SendingShedule(chatID, "231(2)", "Понедельник", "scheduleFor")

	case "tuesday2312":
		SendingShedule(chatID, "231(2)", "Вторник", "scheduleFor")

	case "wednesday2312":
		SendingShedule(chatID, "231(2)", "Среда", "scheduleFor")

	case "thursday2312":
		SendingShedule(chatID, "231(2)", "Четверг", "scheduleFor")

	case "friday2312":
		SendingShedule(chatID, "231(2)", "Пятница", "scheduleFor")

	// Расписание на разные дни недели для 232(1).
	case "232(1)weekday":
		var replyMarkup tgbotapi.InlineKeyboardMarkup

		btn := tgbotapi.NewInlineKeyboardButtonData("Понедельник", "monday2321")
		btn1 := tgbotapi.NewInlineKeyboardButtonData("Вторник", "tuesday2321")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("Среда", "wednesday2321")
		btn3 := tgbotapi.NewInlineKeyboardButtonData("Четверг", "thursday2321")
		btn4 := tgbotapi.NewInlineKeyboardButtonData("Пятница", "friday2321")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn),
			tgbotapi.NewInlineKeyboardRow(btn1),
			tgbotapi.NewInlineKeyboardRow(btn2),
			tgbotapi.NewInlineKeyboardRow(btn3),
			tgbotapi.NewInlineKeyboardRow(btn4),
		)

		msg := tgbotapi.NewMessage(chatID, "Выберите день недели:")
		msg.ReplyMarkup = replyMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "monday2321":
		SendingShedule(chatID, "232(1)", "Понедельник", "scheduleFor")

	case "tuesday2321":
		SendingShedule(chatID, "232(1)", "Вторник", "scheduleFor")

	case "wednesday2321":
		SendingShedule(chatID, "232(1)", "Среда", "scheduleFor")

	case "thursday2321":
		SendingShedule(chatID, "232(1)", "Четверг", "scheduleFor")

	case "friday2321":
		SendingShedule(chatID, "232(1)", "Пятница", "scheduleFor")

		// Расписание на разные дни недели для 232(2).

	case "232(2)weekday":
		var replyMarkup tgbotapi.InlineKeyboardMarkup

		btn := tgbotapi.NewInlineKeyboardButtonData("Понедельник", "monday2322")
		btn1 := tgbotapi.NewInlineKeyboardButtonData("Вторник", "tuesday2322")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("Среда", "wednesday2322")
		btn3 := tgbotapi.NewInlineKeyboardButtonData("Четверг", "thursday2322")
		btn4 := tgbotapi.NewInlineKeyboardButtonData("Пятница", "friday2322")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn),
			tgbotapi.NewInlineKeyboardRow(btn1),
			tgbotapi.NewInlineKeyboardRow(btn2),
			tgbotapi.NewInlineKeyboardRow(btn3),
			tgbotapi.NewInlineKeyboardRow(btn4),
		)

		msg := tgbotapi.NewMessage(chatID, "Выберите день недели:")
		msg.ReplyMarkup = replyMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "monday2322":
		SendingShedule(chatID, "232(2)", "Понедельник", "scheduleFor")

	case "tuesday2322":
		SendingShedule(chatID, "232(2)", "Вторник", "scheduleFor")

	case "wednesday2322":
		SendingShedule(chatID, "232(2)", "Среда", "scheduleFor")

	case "thursday2322":
		SendingShedule(chatID, "232(2)", "Четверг", "scheduleFor")

	case "friday2322":
		SendingShedule(chatID, "232(2)", "Пятница", "scheduleFor")

		// Расписание на разные дни недели для 233(1).

	case "233(1)weekday":
		var replyMarkup tgbotapi.InlineKeyboardMarkup

		btn := tgbotapi.NewInlineKeyboardButtonData("Понедельник", "monday2331")
		btn1 := tgbotapi.NewInlineKeyboardButtonData("Вторник", "tuesday2331")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("Среда", "wednesday2331")
		btn3 := tgbotapi.NewInlineKeyboardButtonData("Четверг", "thursday2331")
		btn4 := tgbotapi.NewInlineKeyboardButtonData("Пятница", "friday2331")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn),
			tgbotapi.NewInlineKeyboardRow(btn1),
			tgbotapi.NewInlineKeyboardRow(btn2),
			tgbotapi.NewInlineKeyboardRow(btn3),
			tgbotapi.NewInlineKeyboardRow(btn4),
		)

		msg := tgbotapi.NewMessage(chatID, "Выберите день недели:")
		msg.ReplyMarkup = replyMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "monday2331":
		SendingShedule(chatID, "233(1)", "Понедельник", "scheduleFor")

	case "tuesday2331":
		SendingShedule(chatID, "233(1)", "Вторник", "scheduleFor")

	case "wednesday2331":
		SendingShedule(chatID, "233(1)", "Среда", "scheduleFor")

	case "thursday2331":
		SendingShedule(chatID, "233(1)", "Четверг", "scheduleFor")

	case "friday2331":
		SendingShedule(chatID, "233(1)", "Пятница", "scheduleFor")

		// Расписание на разные дни недели для 233(2).

	case "233(2)weekday":
		var replyMarkup tgbotapi.InlineKeyboardMarkup

		btn := tgbotapi.NewInlineKeyboardButtonData("Понедельник", "monday2332")
		btn1 := tgbotapi.NewInlineKeyboardButtonData("Вторник", "tuesday2332")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("Среда", "wednesday2332")
		btn3 := tgbotapi.NewInlineKeyboardButtonData("Четверг", "thursday2332")
		btn4 := tgbotapi.NewInlineKeyboardButtonData("Пятница", "friday2332")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn),
			tgbotapi.NewInlineKeyboardRow(btn1),
			tgbotapi.NewInlineKeyboardRow(btn2),
			tgbotapi.NewInlineKeyboardRow(btn3),
			tgbotapi.NewInlineKeyboardRow(btn4),
		)

		msg := tgbotapi.NewMessage(chatID, "Выберите день недели:")
		msg.ReplyMarkup = replyMarkup
		_, err = bot.Send(msg)
		if err != nil {
			log.Println(err)
		}

	case "monday2332":
		SendingShedule(chatID, "233(2)", "Понедельник", "scheduleFor")

	case "tuesday2332":
		SendingShedule(chatID, "233(2)", "Вторник", "scheduleFor")

	case "wednesday2332":
		SendingShedule(chatID, "233(2)", "Среда", "scheduleFor")

	case "thursday2332":
		SendingShedule(chatID, "233(2)", "Четверг", "scheduleFor")

	case "friday2332":
		SendingShedule(chatID, "233(2)", "Пятница", "scheduleFor")

	case "leave_comment":
		Message := "Чтобы оставить комментарий, вызовите /leave_comment."
		msg := tgbotapi.NewMessage(chatID, Message)
		_, err := bot.Send(msg)
		if err != nil {
			log.Println(err)
		}
	}
}

func UpdateUserName(chatID int64, name string) error {
	if globalSessions == nil {
		globalSessions = make(Sessions)
	}

	// Получаем пользователя по chatID.
	user, err := GetUserByChatID(chatID)
	if err != nil {
		// Ошибка при получении пользователя, возвращаем её.
		return err
	}

	// Обновляем имя пользователя напрямую в globalSessions.
	user.Name = name               // Обновляем структуру User, но это копия.
	globalSessions[chatID] = *user // Обновляем сессию в globalSessions, чтобы изменения сохранялись.
	// Возвращаем nil, т.к. ошибок нет и пользователь обновлен успешно.

	resp, err := http.Get("http://localhost:8080/update?Id=" + url.QueryEscape(user.Git_id) + "&Key=Username" + "&Value=" + url.QueryEscape(name))

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Проверяем успешность запроса (код 200)
	if resp.StatusCode != http.StatusOK {
		fmt.Errorf("неправильный статус, код: %d", resp.StatusCode)
	}

	return nil
}

func UpdateUserGroup(chatID int64, group string) error {
	if globalSessions == nil {
		globalSessions = make(Sessions)
	}

	// Получаем пользователя по chatID.
	user, err := GetUserByChatID(chatID)
	if err != nil {
		// Ошибка при получении пользователя, возвращаем её.
		return err
	}

	// Обновляем имя пользователя напрямую в globalSessions.
	user.Group = group
	globalSessions[chatID] = *user // Обновляем сессию в globalSessions, чтобы изменения сохранялись.

	resp, err := http.Get("http://localhost:8080/update?Id=" + url.QueryEscape(user.Git_id) + "&Key=Group" + "&Value=" + url.QueryEscape(group))

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Проверяем успешность запроса (код 200)
	if resp.StatusCode != http.StatusOK {
		fmt.Errorf("неправильный статус, код: %d", resp.StatusCode)
	}

	return nil
}

func getMainKeyboard(role string) tgbotapi.InlineKeyboardMarkup {
	var replyMarkup tgbotapi.InlineKeyboardMarkup

	if role == "student" {
		btn1 := tgbotapi.NewInlineKeyboardButtonData("Учеба", "education")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("Изменить информацию о себе", "about_you")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn1),
			tgbotapi.NewInlineKeyboardRow(btn2),
		)
	} else if role == "teacher" {
		btn1 := tgbotapi.NewInlineKeyboardButtonData("Учеба", "education")
		btn2 := tgbotapi.NewInlineKeyboardButtonData("Изменить информацию о себе", "about_you2")

		replyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(btn1),
			tgbotapi.NewInlineKeyboardRow(btn2),
		)
	}

	return replyMarkup
}

func startNewAdminSession(chatID int64, jwtToken string) (string, error) {
	url := fmt.Sprintf("http://localhost:8082/new_session?id=%s&jwt_token=%s", chatID, jwtToken)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}
	return string(body), nil
}

func getSheduleOutputByDay(jwt string, group string, day string, actionCode string) (string, error) {
	resp, err := http.Get(server + "/getSchedule?JWTtoken=" + url.QueryEscape(jwt) + "&group=" + url.QueryEscape(group) + "&day=" + url.QueryEscape(day) + "&actionCode=" + url.QueryEscape(actionCode))
	if err != nil {
		fmt.Println("Ошибка: ", err)
	}
	defer resp.Body.Close()

	// Проверяем успешность запроса (код 200)
	if resp.StatusCode != http.StatusOK {
		fmt.Errorf("неправильный статус, код: %d", resp.StatusCode)
	}

	// Читаем тело ответа
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
	}

	return string(body), err
}

func whereIsTeacher(teacher string, lessonNumber string) (string, error) {
	resp, err := http.Get(server + "/WhereTeacher?Teacher=" + url.QueryEscape(teacher) + "&LessonIndex=" + url.QueryEscape(lessonNumber))
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()

	// Проверяем успешность запроса (код 200)
	if resp.StatusCode != http.StatusOK {
		fmt.Errorf("неправильный статус, код: %d", resp.StatusCode)
	}

	// Читаем тело ответа
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	log.Println("сработала whereIsTeacher", string(body))

	return string(body), err
}

func whereIsGroup(numberLesson string, group string) (string, error) {
	resp, err := http.Get(server + "/WhereGroup?LessonIndex=" + url.QueryEscape(numberLesson) + "&group=" + url.QueryEscape(group))
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()

	// Проверяем успешность запроса (код 200)
	if resp.StatusCode != http.StatusOK {
		fmt.Errorf("неправильный статус, код: %d", resp.StatusCode)
	}

	// Читаем тело ответа
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	return string(body), err
}

func SendingShedule(chatID int64, group string, day string, actionCode string) {
	user, err := GetUserByChatID(chatID)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, "Вы еще не авторизовались, пожалуйста, авторизуйтесь /login.")
		_, err := bot.Send(msg)
		if err != nil {
			log.Println(err)
		}
		return
	}

	named := IsNamed(chatID)

	if named == false && user.Role == "student" {
		msg := tgbotapi.NewMessage(chatID, "Вы еще не назвались, пожалуйста, назовитесь /whoami.")
		_, err := bot.Send(msg)
		if err != nil {
			log.Println(err)
		}
		return
	}

	jwt, err := getJWTToken(chatID)
	if err != nil {
		fmt.Println(err)
	}
	output, err1 := getSheduleOutputByDay(jwt, group, day, actionCode)
	if err1 != nil {
		fmt.Println(err1)
	}

	msg := tgbotapi.NewMessage(chatID, output)
	_, err2 := bot.Send(msg)
	if err2 != nil {
		log.Println(err2)
	}
}

func leaveComment(jwt string, group string, lessonNumber string, comment string, day string, teacher string) (string, error) {
	resp, err := http.Get(server + "/AddCommentary?JWTtoken=" + url.QueryEscape(jwt) + "&group=" + url.QueryEscape(group) + "&LessonIndex=" + url.QueryEscape(lessonNumber) + "&Commentary=" + url.QueryEscape(comment) + "&Day=" + url.QueryEscape(day) + "&Teacher=" + url.QueryEscape(teacher))
	if err != nil {
		fmt.Println("Ошибка: ", err)
	}
	defer resp.Body.Close()

	// Проверяем успешность запроса (код 200)
	if resp.StatusCode != http.StatusOK {
		fmt.Errorf("неправильный статус, код: %d", resp.StatusCode)
	}

	// Читаем тело ответа
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	return string(body), err
}

func sendToken(jwt string, token string) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:8082/new_session?id=%s&jwt_token=%s", token, jwt)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	return string(body), nil
}

func nextLessonForStudent(numberLesson string, group string) (string, error) {
	resp, err := http.Get(server + "/NextLessonForStudent?LessonIndex=" + url.QueryEscape(numberLesson) + "&group=" + url.QueryEscape(group))
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()

	// Проверяем успешность запроса (код 200)
	if resp.StatusCode != http.StatusOK {
		fmt.Errorf("неправильный статус, код: %d", resp.StatusCode)
	}

	// Читаем тело ответа
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
	}

	return string(body), err
}

func nextLessonForTeacher(numberLesson string, teacher string) (string, error) {
	resp, err := http.Get(server + "/NextLessonForTeacher?LessonIndex=" + url.QueryEscape(numberLesson) + "&Teacher=" + url.QueryEscape(teacher))
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()

	// Проверяем успешность запроса (код 200)
	if resp.StatusCode != http.StatusOK {
		fmt.Errorf("неправильный статус, код: %d", resp.StatusCode)
	}

	// Читаем тело ответа
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	return string(body), err
}
