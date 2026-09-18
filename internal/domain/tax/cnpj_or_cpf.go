package tax

import (
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/andreis3/isura-ledger-ms/internal/domain/validator"
)

var blackListCNPJ = []string{
	"00000000000000", "11111111111111", "22222222222222", "33333333333333",
	"44444444444444", "55555555555555", "66666666666666", "77777777777777",
	"88888888888888", "99999999999999",
}

const (
	CPFLength            = 11
	CPFFirstDigitIdx     = 9
	CPFSecondDigitIdx    = 10
	CPFModuleDivisor     = 11
	CPFBlacklistLength   = 10
	CPFASCIIZero         = '0'
	CPFFirstDigitWeight  = 10
	CPFSecondDigitWeight = 11
)

var blackListCPF = []string{
	"00000000000", "11111111111", "22222222222", "33333333333",
	"44444444444", "55555555555", "66666666666", "77777777777",
	"88888888888", "99999999999",
}

type CNPJOrCPF struct {
	value string
}

// NewCNPJOrCPF tries to create the Value Object with optimized validations.
func NewCNPJOrCPF(rawTaxID string) (*CNPJOrCPF, validator.Evaluator) {
	var eval validator.Evaluator

	cleanCNPJOrCPF := cleanCONPJOrCPF(rawTaxID)

	eval.CheckField(validator.NotBlank(cleanCNPJOrCPF), "tax_id", "cannot be blank")

	if len(cleanCNPJOrCPF) == 14 {
		eval.CheckField(!slices.Contains(blackListCNPJ, cleanCNPJOrCPF), "tax_id", "must be a valid CNPJ number")
		eval.CheckField(validateCNPJ(cleanCNPJOrCPF), "tax_id", "invalid digits calculated with module 11 algorithm")
	}

	if len(cleanCNPJOrCPF) == 11 {
		eval.CheckField(!slices.Contains(blackListCPF, cleanCNPJOrCPF), "tax_id", "must be a valid CPF number")
		eval.CheckField(validateCPF(cleanCNPJOrCPF), "tax_id", "invalid digits calculated with module 10 algorithm")
	}

	if len(eval) > 0 {
		return nil, eval
	}

	return &CNPJOrCPF{value: cleanCNPJOrCPF}, eval
}

func cleanCONPJOrCPF(cpf string) string {
	var sb strings.Builder
	// Pre-allocates the estimated maximum size to avoid buffer reallocations
	sb.Grow(len(cpf))
	for _, r := range cpf {
		if unicode.IsDigit(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func (c *CNPJOrCPF) String() string {
	return c.value
}

func validateCNPJ(cnpj string) bool {
	size := len(cnpj) - 2
	numbers := cnpj[:size]
	digits := cnpj[size:]
	sum := 0
	pos := size - 7

	for i := size; i >= 1; i-- {
		// Optimization: Direct ASCII subtraction instead of strconv.Atoi
		convertNumber := int(numbers[size-i] - CPFASCIIZero)
		sum += convertNumber * pos
		pos--
		if pos < 2 {
			pos = 9
		}
	}

	result := 0
	if rest := sum % 11; rest >= 2 {
		result = 11 - rest
	}

	if strconv.Itoa(result) != string(digits[0]) {
		return false
	}

	size++
	numbers = cnpj[:size]
	sum = 0
	pos = size - 7

	for i := size; i >= 1; i-- {
		convertNumber := int(numbers[size-i] - CPFASCIIZero)
		sum += convertNumber * pos
		pos--
		if pos < 2 {
			pos = 9
		}
	}

	result = 0
	if rest := sum % 11; rest >= 2 {
		result = 11 - rest
	}

	if strconv.Itoa(result) != string(digits[1]) {
		return false
	}

	return true
}

func validateCPF(cpf string) bool {
	if len(cpf) != CPFLength || slices.Contains(blackListCPF, cpf) {
		return false
	}
	return validateDigit(cpf, CPFFirstDigitIdx, CPFFirstDigitWeight) &&
		validateDigit(cpf, CPFSecondDigitIdx, CPFSecondDigitWeight)
}

func validateDigit(cpf string, position, startWeight int) bool {
	sum := 0
	for i, char := range cpf[:position] {
		sum += int(char-CPFASCIIZero) * (startWeight - i)
	}

	rest := (sum * 10) % CPFModuleDivisor
	if rest == CPFBlacklistLength {
		rest = 0
	}

	return rest == int(cpf[position]-CPFASCIIZero)
}
