package syntax

import "fmt"

// token.go is the in-house token set the parser emits and the evaluators
// switch on. It replaces cuelang.org/go/cue/token (plan 240): only the tokens
// the cuelite subset reaches are defined, with the SAME names the consumers
// already use (token.MUL, token.OR, token.GEQ, …) so a consumer's switch
// changes only its import.

// Token is one lexical token or operator. The String method renders an
// operator as its source spelling for the error messages that quote it
// (`cuelite: unsupported binary operator "&&"`).
type Token int

const (
	// NoToken is the zero value: a Field with no `?` constraint carries it, so
	// `el.Constraint == OPTION` is false for a required field.
	NoToken Token = iota

	// Literal-kind tokens (BasicLit.Kind).
	STRING       // a quoted string literal
	INT          // an integer literal
	FLOAT        // a floating-point literal
	TRUE         // the bool literal true
	FALSE        // the bool literal false
	NULL         // the null literal
	kInterpFrag  // an Interpolation fragment whose Value is already decoded

	// Structural tokens.
	OPTION // the `?` optional-field marker (Field.Constraint)

	// Binary / unary operators.
	OR   // |  disjunction
	AND  // &  meet
	ADD  // +  addition / unary plus
	SUB  // -  subtraction / unary minus
	MUL  // *  multiplication / disjunction default mark
	NOT  // !  boolean negation
	EQL  // == equal
	NEQ  // != not equal
	LSS  // <  less
	GTR  // >  greater
	LEQ  // <= less or equal
	GEQ  // >= greater or equal
	MAT  // =~ regex match
	NMAT // !~ regex non-match
)

// String renders a token as its source spelling, for error messages that
// quote the operator. A non-operator token renders as a name.
func (t Token) String() string {
	switch t {
	case OR:
		return "|"
	case AND:
		return "&"
	case ADD:
		return "+"
	case SUB:
		return "-"
	case MUL:
		return "*"
	case NOT:
		return "!"
	case EQL:
		return "=="
	case NEQ:
		return "!="
	case LSS:
		return "<"
	case GTR:
		return ">"
	case LEQ:
		return "<="
	case GEQ:
		return ">="
	case MAT:
		return "=~"
	case NMAT:
		return "!~"
	case OPTION:
		return "?"
	case STRING:
		return "string-literal"
	case INT:
		return "int-literal"
	case FLOAT:
		return "float-literal"
	case TRUE:
		return "true"
	case FALSE:
		return "false"
	case NULL:
		return "null"
	default:
		return fmt.Sprintf("token(%d)", int(t))
	}
}
