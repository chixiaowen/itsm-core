// Package statemachine 提供与业务无关的通用状态机引擎。
//
// 每个业务域在自己的 machine.go 中声明一张 Table（from -> action -> Transition），
// 由 service 层调用 Resolve / Allows 完成「非法流转 -> 409」「越权 -> 403」判定。
package statemachine

import (
	"context"
	"time"
)

// Actor 是当前操作者。
type Actor struct {
	UserID uint64
	Role   string
}

// GuardInput 是前置校验入参。
type GuardInput struct {
	// Ctx 请求上下文。
	Ctx context.Context
	// Actor 当前操作者。
	Actor Actor
	// Entity 当前实体指针（如 *ticket.Ticket）。
	Entity any
	// Params 请求附加参数（solution/reason/assignee_id...）。
	Params map[string]any
	// Now 当前时间（由 service 注入，便于测试）。
	Now time.Time
}

// Guard 是前置校验函数；返回非 nil 表示拒绝（由 service 转成 422）。
type Guard func(in GuardInput) error

// Transition 是一条允许的流转。
type Transition struct {
	// To 目标状态。
	To string
	// Roles 允许角色；为空表示不限制角色。
	Roles []string
	// Guard 可选前置校验。
	Guard Guard
}

// Table 是状态机表：from -> action -> Transition。
type Table map[string]map[string]Transition

// Resolve 查询某个状态下 action 是否被允许。
//
// ok=false 表示非法流转（service 应转成 409）。
func (t Table) Resolve(from, action string) (Transition, bool) {
	byAction, ok := t[from]
	if !ok {
		return Transition{}, false
	}
	tr, ok := byAction[action]
	return tr, ok
}

// Allows 校验角色是否允许执行该流转；Roles 为空视为放行。
func (tr Transition) Allows(role string) bool {
	if len(tr.Roles) == 0 {
		return true
	}
	for _, r := range tr.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Actions 返回某状态下所有允许的 action 名（无序）。
//
// 供前端详情页渲染操作按钮组使用；后端仍以 Resolve 强校验。
func (t Table) Actions(from string) []string {
	byAction, ok := t[from]
	if !ok {
		return nil
	}
	actions := make([]string, 0, len(byAction))
	for a := range byAction {
		actions = append(actions, a)
	}
	return actions
}
