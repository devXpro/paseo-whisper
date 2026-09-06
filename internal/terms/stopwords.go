package terms

// Words that carry no domain meaning. Kept deliberately broad: a missed term
// costs one line in the output, a leaked common word wastes prompt budget that
// the model only has 224 tokens of.
var stopWords = map[string]bool{}

func init() {
	for _, list := range [][]string{english, russian, harness} {
		for _, w := range list {
			stopWords[w] = true
		}
	}
}

var english = []string{
	"the", "and", "for", "you", "are", "that", "this", "with", "have", "not", "but",
	"can", "will", "your", "from", "they", "what", "when", "where", "which", "who",
	"how", "why", "all", "any", "some", "more", "most", "other", "into", "than",
	"then", "them", "there", "these", "those", "was", "were", "been", "being", "had",
	"has", "does", "did", "doing", "done", "just", "like", "make", "made", "get",
	"got", "give", "given", "take", "taken", "want", "need", "also", "only", "very",
	"much", "many", "few", "own", "same", "such", "about", "after", "again", "because",
	"before", "below", "between", "both", "during", "each", "further", "here", "once",
	"over", "under", "until", "while", "would", "could", "should", "shall", "may",
	"might", "must", "let", "lets", "please", "thanks", "yes", "yeah", "okay",
	"one", "two", "three", "now", "new", "old", "good", "bad", "big", "small",
	"right", "left", "top", "end", "use", "used", "using", "uses", "file", "files",
	"line", "lines", "code", "name", "names", "time", "times", "day", "days",
	"work", "works", "working", "think", "need", "needs", "look", "looks", "see",
	"say", "says", "way", "ways", "run", "runs", "running", "add", "added",
}

// harness words come from tooling that injects text into transcripts; they are
// not something the user ever says out loud.
var harness = []string{
	"command", "output", "status", "summary", "tool", "tools", "task", "tasks",
	"message", "messages", "response", "background", "completed", "interrupted",
	"notification", "args", "caveat", "unless", "explicitly", "asks", "otherwise",
	"consider", "generated", "tmp", "private", "exit", "stdout", "stderr",
	"users", "home", "http", "https", "com", "org", "net", "www", "null", "true",
	"false", "error", "warn", "info", "debug", "log", "logs",
}

var russian = []string{
	"этот", "этого", "этому", "этим", "тебе", "меня", "себя", "нужно", "надо",
	"можно", "нельзя", "давай", "давайте", "сделай", "сделал", "делать", "сделать",
	"посмотри", "смотри", "понял", "понятно", "короче", "просто", "может", "можешь",
	"должен", "должно", "должна", "должны", "если", "чтобы", "когда", "потому",
	"такой", "такая", "такое", "такие", "который", "которые", "которая", "всё",
	"ещё", "еще", "уже", "теперь", "опять", "хорошо", "плохо", "нормально",
	"попробуй", "попробуем", "проверь", "проверим", "значит", "вообще", "только",
	"очень", "сейчас", "потом", "сначала", "после", "перед", "через", "между",
	"самый", "самое", "почему", "зачем", "откуда", "куда", "сколько", "какой",
	"какая", "какие", "больше", "меньше", "лучше", "хуже", "быстро", "медленно",
	"много", "мало", "немного", "слишком", "тоже", "также", "кстати", "вроде",
	"кажется", "наверное", "конечно", "точно", "именно", "даже", "почти", "совсем",
	"никак", "никогда", "всегда", "знаю", "знаешь", "думаю", "думаешь", "хочу",
	"хочешь", "буду", "будет", "будем", "были", "было", "есть", "нету", "пиши",
	"пишет", "напиши", "написал", "говорю", "сказал", "скажи", "слушай", "спасибо",
	"пожалуйста", "извини", "ладно", "окей", "нужен", "нужна", "нужны", "погоди",
	"понимаю", "дальше", "ничего", "можем", "добавить", "будут", "сразу",
	"насколько", "погнали", "вопрос", "настройки", "сделаем", "делай", "работы",
	"работать", "работу", "тогда", "разные", "команду", "одной", "например",
	"продолжай", "работает", "происходит", "поэтому", "поставить", "новый",
	"будешь", "рядом", "вначале", "пользователь", "пусть", "время", "сегодня",
	"делал", "других", "писать", "возможно", "задача", "новые", "видеть",
	"стоит", "допустим", "правильно", "делаем", "папку", "запускать",
	"чтоб", "типа", "пока", "быть", "один", "одна", "одно", "тебя", "этом",
	"того", "туда", "сюда", "этой", "этих", "этот", "вижу", "него", "неё",
	"свой", "своя", "свои", "была", "были", "проблема", "проблемы", "нету",
	"вроде", "может", "можем", "хочет", "давно", "снова", "опять", "везде",
	"нигде", "здесь", "оттуда", "потом", "затем", "далее", "выше", "ниже",
	"первый", "второй", "третий", "последний", "следующий", "предыдущий",
	"вместо", "кроме", "около", "внутри", "снаружи", "вместе", "отдельно",
	"обычно", "иногда", "редко", "часто", "прям", "прямо", "почему-то",
	"какой-то", "что-то", "как-то", "где-то", "кто-то", "чего-то", "нибудь",
	"обязательно", "каких", "хотя", "всего", "написать", "нужное", "самом",
	"деле", "конце", "начале", "случае", "момент", "раньше", "позже",
}
