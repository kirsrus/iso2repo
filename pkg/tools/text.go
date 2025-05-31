package tools

import "fmt"

// LogInfoWidget выравнивает и обрамляет текст из text, возвращая его в formatedText и
// обёртывая в бордер из символа border
func LogInfoWidget(text []string, border string) (formatedText []string) {

	if len(border) == 0 {
		border = "*"
	} else if len(border) > 1 {
		border = border[0:1]
	}

	maxLen := 0
	for _, v := range text {
		if len([]rune(v)) > maxLen {
			maxLen = len([]rune(v))
		}
	}

	borderTopBottom := ""

	for i := 0; i < maxLen+4; i++ {
		borderTopBottom += border
	}

	formatedText = append(formatedText, borderTopBottom)
	for _, v := range text {
		for i := len([]rune(v)); i < maxLen; i++ {
			v += " "
		}
		formatedText = append(formatedText, fmt.Sprintf("%s %s %s", border, v, border))
	}
	formatedText = append(formatedText, borderTopBottom)

	return formatedText
}
