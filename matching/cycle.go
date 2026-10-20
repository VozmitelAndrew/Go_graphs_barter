package matching

import "slices"

type color uint8

const (
	white color = iota
	gray
	black
)

func FindCycle(adj [][]int) []int {
	state := make([]color, len(adj))
	pos := make([]int, len(adj))
	var stack []int

	var dfs func(u int) []int

	dfs = func(u int) []int {
		state[u] = gray
		pos[u] = len(stack)
		stack = append(stack, u)

		for _, v := range adj[u] {
			switch state[v] {
			case white:
				if c := dfs(v); c != nil {
					return c
				}
			case gray:
				return slices.Clone(stack[pos[v]:])
			}
		}

		stack = stack[:len(stack)-1]
		state[u] = black
		return nil
	}

	//rn looking for 1 cycle, later will think about a better solution
	for u := range adj {
		if state[u] == white {
			if c := dfs(u); c != nil {
				return c
			}
		}
	}
	return nil
}
