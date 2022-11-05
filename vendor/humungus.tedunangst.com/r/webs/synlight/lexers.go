package synlight

var lexer_c = "" +
	"whitespace [ \\t\\r\\n]+\n" +
	"number (0[xX][0-9a-fA-F]+|[0-9]+)\n" +
	"number '(\\\\.|[^'])'\n" +
	"keyword (if|else|for|while|continue|break|switch|case|default|return|typedef|extern)\\b\n" +
	"builtin (NULL|va_list|true|false)\\b\n" +
	"builtin #(\\\\\\n|[^\\n])*\n" +
	"type (static|struct|const|unsigned|char|int|long|byte|bool|void|int8_t|uint8_t|int32_t|uint32_t)\\b\n" +
	"string \"(\\\\\"|[^\"])*\"\n" +
	"word [a-zA-Z_][a-zA-Z0-9_]*\n" +
	"comment //[^\\n]*\n" +
	"comment /\\*(?s:.*?)\\*/\n" +
	"operator [-+/*=:<>,\\.!]+\n" +
	"pair [([{]\n" +
	"unpair [)\\]}]\n" +
	""
var lexer_diff = "" +
	"keyword diff[^\\n]*\n" +
	"builtin @@[^\\n]*\n" +
	"delline -[^\\n]*\n" +
	"addline \\+[^\\n]*\n" +
	"text [^\\n]+\n" +
	"newline \\n\n" +
	""
var lexer_go = "" +
	"whitespace [ \\t\\r\\n]+\n" +
	"number (0[xX][0-9a-fA-F]+|[0-9]+)\n" +
	"number '(\\\\.|[^'])'\n" +
	"keyword (package|if|else|nil|func|var|for|continue|break|switch|case|default|return|type)\\b\n" +
	"builtin (import|defer|len|append|range|make|true|false)\\b\n" +
	"type (struct|interface|string|map|int|byte|bool|chan|int32)\\b\n" +
	"string `[^`]*`\n" +
	"string \"(\\\\\"|[^\"])*\"\n" +
	"word [a-zA-Z_][a-zA-Z0-9_]*\n" +
	"comment //[^\\n]*\n" +
	"comment /\\*(?s:.*?)\\*/\n" +
	"operator [-+/*=:<>,\\.!]+\n" +
	"nop (\\[]|{})\n" +
	"pair [([{]\n" +
	"unpair [)\\]}]\n" +
	""
var lexer_html = "" +
	"builtin:0:1 <[/a-zA-Z]*\n" +
	"keyword:1:1 [^=/>\"]+\n" +
	"string:1:1 \"[^\"]*\"\n" +
	"text:1:1 [\\s=]+\n" +
	"builtin:1:0 /?>\n" +
	"comment:0:0 {{(?s:.*?)}}\n" +
	"text:0:0 [^<{]+\n" +
	""
var lexer_js = "" +
	"whitespace [ \\t\\r\\n]+\n" +
	"number (0[xX][0-9a-fA-F]+|[0-9]+)\n" +
	"keyword (function|return|var|break|and|or|not|do|else|if|then|elseif|for|in|while|end)\\b\n" +
	"builtin (new|escape|true|false)\\b\n" +
	"builtin ((console|Math|window|document|history)\\.[[:alpha:]]+)\\b\n" +
	"string \\[=*\\[(?s:.*?)\\]=*\\]\n" +
	"string '(\\\\'|[^'])*'\n" +
	"string \"(\\\\\"|[^\"])*\"\n" +
	"word [a-zA-Z_][a-zA-Z0-9_]*\n" +
	"comment //[^\\n]*\n" +
	"operator [-+/*=:<>,\\.!]+\n" +
	"pair [([{]\n" +
	"unpair [)\\]}]\n" +
	""
var lexer_lua = "" +
	"whitespace [ \\t\\r\\n]+\n" +
	"number (0[xX][0-9a-fA-F]+|[0-9]+)\n" +
	"keyword (function|return|local|break|and|or|not|do|else|if|then|elseif|for|in|while|end)\\b\n" +
	"builtin (require|print|pairs|ipairs|next|true|false)\\b\n" +
	"builtin (string\\.format)\\b\n" +
	"string \\[=*\\[(?s:.*?)\\]=*\\]\n" +
	"string '(\\\\'|[^'])*'\n" +
	"string \"(\\\\\"|[^\"])*\"\n" +
	"word [a-zA-Z_][a-zA-Z0-9_]*\n" +
	"comment --[^\\n]*\n" +
	"operator [-+/*=:<>,\\.!]+\n" +
	"pair [([{]\n" +
	"unpair [)\\]}]\n" +
	""
var lexer_py = "" +
	"whitespace [ \\t\\r\\n]+\n" +
	"number (0[xX][0-9a-fA-F]+|[0-9]+)\n" +
	"keyword (def|return|break|or|and|not|do|else|is|if|then|elif|try|raise|except|for|in|while|as)\\b\n" +
	"builtin (from|import|print|isinstance|None|True|False|Exception)\\b\n" +
	"string '(\\\\'|[^'])*'\n" +
	"string \"(\\\\\"|[^\"])*\"\n" +
	"word [a-zA-Z_][a-zA-Z0-9_]*\n" +
	"comment #[^\\n]*\n" +
	"operator [-+/*=:[\\](){}<>,\\.!]+\n" +
	""
var lexer_rs = "" +
	"whitespace [ \\t\\r\\n]+\n" +
	"number (0[xX][0-9a-fA-F]+|[0-9]+)\n" +
	"number '(\\\\.|[^'])'\n" +
	"keyword (extern|if|else|fn|pub|let|for|loop|continue|break|match|return|type|struct|impl)\\b\n" +
	"builtin (Err|Ok|Result|Some|None|dyn|unsafe|len|new|true|false|unwrap|as_bytes|to_string|as_str)\\b\n" +
	"builtin (println!|format!)\n" +
	"type (&?mut|const|&?String|i32|u32|i8|u8|i64|byte|bool|chan|usize)\\b\n" +
	"type (\\w+::)\n" +
	"string ^#[^\\n]*\n" +
	"builtin ^use [^\\n]*\n" +
	"string `[^`]*`\n" +
	"string \"(\\\\\"|[^\"])*\"\n" +
	"word [a-zA-Z_][a-zA-Z0-9_]*\n" +
	"comment //[^\\n]*\n" +
	"comment /\\*(?s:.*?)\\*/\n" +
	"operator [-+/*=:<>,\\.!]+\n" +
	"pair [([{]\n" +
	"unpair [)\\]}]\n" +
	""
var lexer_sql = "" +
	"whitespace [ \\t\\r\\n]+\n" +
	"number (0[xX][0-9a-fA-F]+|[0-9]+)\n" +
	"keyword ((?i)create|using|insert|into|select|from|on|where|join|like)\\b\n" +
	"builtin ((?i)table|index|values|integer|primary|key|blob|text)\\b\n" +
	"string \\[=*\\[(?s:.*?)\\]=*\\]\n" +
	"string '(''|[^'])*'\n" +
	"word [a-zA-Z_][a-zA-Z0-9_]*\n" +
	""
