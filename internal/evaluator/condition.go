package evaluator

import "github.com/ding-labs/ding/internal/condition"

// Legacy aliases keep the original behavior oracle while the watch runtime uses
// the extracted pure grammar directly. Removed at the architectural cutover.
type Condition = condition.Condition
type ConditionExpr = condition.ConditionExpr
type evalContext = condition.Context
type windowedLeaf = condition.Window

func ParseCondition(s string) (Condition, error)         { return condition.ParseCondition(s) }
func ParseConditionExpr(s string) (ConditionExpr, error) { return condition.ParseExpression(s) }
