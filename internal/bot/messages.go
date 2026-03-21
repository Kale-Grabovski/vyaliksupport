package bot

// btnLabelHome and btnLabelFAQ are the persistent reply-keyboard button labels.
// They must match exactly what the user sends when tapped.
const (
	btnLabelHome = "🏠 Главная"
	btnLabelFAQ  = "❓ FAQ"
)

const (
	// msgWelcome uses HTML parse mode — safe for any username with underscores.
	msgWelcome = `Привет! Я бот поддержки <a href="https://t.me/VPNinjas_bot">VPNinjas</a>.

Если у тебя вопрос — нажми на FAQ или опиши проблему текстом.
Чем подробнее — тем быстрее поможем.`

	msgFAQ = `*Часто задаваемые вопросы:*

Выбери нужный раздел 👇`

	msgUnsupportedType = `📎 [Неподдерживаемый тип сообщения]`

	msgSentToSupport = `✅ Сообщение поддержке отправлено. Ответим в ближайшее время.`

	msgReplyNotFound = `❌ Не могу найти пользователя по этому сообщению`

	msgReplySentOK = `✅ Ответ отправлен пользователю.`

	msgReplyUserBlocked = `❌ Не удалось отправить ответ пользователю. Возможно, он заблокировал бота.`

	msgReplyHeader = `👨‍💻 Ответ из поддержки:`
)

// faqItem describes one FAQ entry shown as an inline button.
type faqItem struct {
	// Button label visible to the user.
	Label string
	// Full answer text sent when the button is pressed.
	Answer string
}

// faqItems is the ordered list of FAQ entries rendered as inline buttons.
var faqItems = []faqItem{
	{
		Label: "🔌 Как подключиться?",
		Answer: `*Как подключиться?*

1. Скачай клиент под своё устройство:

• *Windows* — Happ / Hiddify
• *Android* — V2rayNG / Happ
• *iOS* — Streisand / FoXray
• *macOS* — Streisand / V2rayU / FoXray
• *Linux* — Happ / Nekoray / Qv2ray

2. Импортируй полученную ссылку на подписку в клиент.
3. Нажми «Подключиться».

Если не получается — опиши проблему текстом, укажи устройство и клиент.`,
	},
	{
		Label: "❌ VPN не работает",
		Answer: `*VPN не работает после подключения*

Попробуй по шагам:

1. Проверь срок подписки — тестовая работает *1 день*, платная — от месяца.
2. Переподключись / перезапусти клиент.
3. Смени сеть: с Wi-Fi на мобильный интернет или наоборот.
4. Проверь, что время на устройстве *синхронизировано* (авто-время).
5. Попробуй другой сервер, если доступен.

Если ничего не помогло — напиши сюда: укажи клиент, устройство и прикрепи скриншот ошибки.`,
	},
	{
		Label: "💳 Как продлить подписку?",
		Answer: `*Как продлить подписку?*

1. Перейди в Мои подписки и выбери нужную.
2. Оплати *150р* или больше на карту или криптой.
3. Подписка продлится автоматически.
4. Если этого не произошло напиши нам чат.

Тестовый период: 1 день, 50 ГБ — после него нужна оплата.`,
	},
	{
		Label: "📱 Какой клиент выбрать?",
		Answer: `*Рекомендуемые клиенты:*

• *Windows* — Happ / Hiddify
• *Android* — V2rayNG / Happ
• *iOS* — Streisand / FoXray
• *macOS* — Streisand / V2rayU / FoXray
• *Linux* — Happ / Nekoray / Qv2ray

Все клиенты бесплатны, ищи в официальных магазинах или на GitHub.`,
	},
	{
		Label: "⚡ Медленная скорость",
		Answer: `*Низкая скорость VPN*

1. Попробуй переподключиться.
2. Смени сеть (Wi-Fi ↔ мобильный интернет).
3. Выбери другую страну из списка.
4. Перезагрузи роутер, если на Wi-Fi.

Если скорость стабильно низкая — напиши нам, разберёмся.`,
	},
	{
		Label: "🔑 Где мой ключ?",
		Answer: `*Где найти свой VPN-ключ?*

Ключом является сама ссылка на подписку и выдаётся после оплаты или активации тестового периода.

Если ссылка не пришла или потерялась — напиши сюда, восстановим.`,
	},
}
