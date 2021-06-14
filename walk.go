package walky

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type WalkStatus int

const (
	WalkExit         WalkStatus = iota
	WalkDepthFirst              // depth-first
	WalkBreadthFirst            // breadth-first
	WalkPrune
)

func (ws WalkStatus) String() string {
	switch ws {
	case WalkExit:
		return "Exit"
	case WalkDepthFirst:
		return "Depth"
	case WalkBreadthFirst:
		return "Breadth"
	case WalkPrune:
		return "Prune"
	default:
		return "Invalid"
	}
}

type WalkFunc func(current, parent *yaml.Node, position int, opts *WalkOptions) (WalkStatus, error)
type NodeFunc func(node *yaml.Node) error

type WalkOptions struct {
	MissStatus  WalkStatus
	matchStatus *WalkStatus
	maxDepth    int
	trace       func(current, parent *yaml.Node, pos, depth int, status WalkStatus, err error)
}

func (opts *WalkOptions) MatchStatus() WalkStatus {
	if opts.matchStatus != nil {
		return *opts.matchStatus
	}
	return opts.MissStatus
}

type WalkOpt func(*WalkOptions)

func WithBreadthFirst() WalkOpt {
	return func(opt *WalkOptions) {
		opt.MissStatus = WalkBreadthFirst
	}
}

func WithFirstOnly() WalkOpt {
	return func(opt *WalkOptions) {
		ws := WalkExit
		opt.matchStatus = &ws
	}
}

func WithMaxDepth(max int) WalkOpt {
	return func(opt *WalkOptions) {
		opt.maxDepth = max
	}
}

func WithTrace(f func(current, parent *yaml.Node, pos, depth int, ws WalkStatus, err error)) WalkOpt {
	return func(opt *WalkOptions) {
		opt.trace = f
	}
}

func Walk(node *yaml.Node, f WalkFunc, walkOpts ...WalkOpt) error {
	opts := &WalkOptions{
		MissStatus: WalkDepthFirst,
		maxDepth:   -1,
	}

	for _, o := range walkOpts {
		o(opts)
	}
	node = unwrapDocument(node)
	ws, err := f(node, nil, -1, opts)
	if opts.trace != nil {
		opts.trace(node, nil, -1, 0, ws, err)
	}
	if err != nil {
		return err
	}
	switch ws {
	case WalkExit:
		return nil
	case WalkPrune:
		return nil
	}

	ws, later, err := walk(node, nil, f, 0, opts)
	if err != nil {
		return nil
	}
	switch ws {
	case WalkExit:
		return nil
	case WalkPrune:
		return nil
	}

	for i := 0; i < len(later); i++ {
		ws, more, err := later[i]()
		if err != nil {
			return nil
		}
		switch ws {
		case WalkExit:
			return nil
		case WalkPrune:
			continue
		}
		later = append(later, more...)
	}
	return nil
}

type nextFunc func() (WalkStatus, []nextFunc, error)

func walk(node, parent *yaml.Node, f WalkFunc, depth int, opts *WalkOptions) (WalkStatus, []nextFunc, error) {
	if opts.maxDepth >= 0 && depth > opts.maxDepth {
		return WalkPrune, nil, nil
	}
	walkLater := []nextFunc{}
	for i := 0; i < len(node.Content); i++ {
		ws, err := f(node.Content[i], node, i, opts)
		if opts.trace != nil {
			opts.trace(node.Content[i], node, i, depth, ws, err)
		}
		if err != nil {
			return ws, nil, err
		}
		subNode := node.Content[i]
		subParent := parent
		// we only call walkFunc on keys of maps, if the walkFunc wants to
		// reference value it will use parent.Content[position+1]
		if node.Kind == yaml.MappingNode {
			// the parent of a map value is the key, not the whole map
			subNode = node.Content[i+1]
			subParent = node.Content[i]
			i++
		}
		switch ws {
		case WalkExit:
			return WalkExit, nil, nil
		case WalkDepthFirst:
			// depth-first
			ws, later, err := walk(subNode, subParent, f, depth+1, opts)
			if err != nil {
				return ws, nil, err
			}
			switch ws {
			case WalkExit:
				return WalkExit, nil, nil
			case WalkPrune:
				break
			}
			walkLater = append(walkLater, later...)
		case WalkBreadthFirst:
			// breadth-first
			walkLater = append(walkLater, func() (WalkStatus, []nextFunc, error) {
				return walk(subNode, subParent, f, depth+1, opts)
			})
		case WalkPrune:
			return WalkDepthFirst, walkLater, nil
		}
	}
	return WalkDepthFirst, walkLater, nil
}

func ScalarValuesWalker(f NodeFunc) WalkFunc {
	return func(current, parent *yaml.Node, pos int, opts *WalkOptions) (ws WalkStatus, err error) {
		if current.Kind != yaml.ScalarNode || parent.Kind == yaml.MappingNode {
			return opts.MissStatus, nil
		}
		err = f(current)
		return opts.MatchStatus(), err
	}
}

// func WalkScalarValues(root *yaml.Node, f func(*yaml.Node) error, opts ...WalkOpt) error {
// 	return Walk(root, ScalarValues(f, opts...))
// }

// func WalkKeyString(root *yaml.Node, key string, f func(*yaml.Node) error, opts ...WalkOpt) error {
// 	return Walk(root, KeyString(key, f, opts...))
// }

// KeyStringWalker is used with Walk to apply `f` to map values that match the
// provided key string.
func KeyStringWalker(key string, f NodeFunc) WalkFunc {
	return func(current, parent *yaml.Node, pos int, opts *WalkOptions) (WalkStatus, error) {
		if parent == nil || parent.Kind != yaml.MappingNode {
			return opts.MissStatus, nil
		}
		if current.Value != key {
			return opts.MissStatus, nil
		}
		err := f(parent.Content[pos+1])
		return opts.MatchStatus(), err
	}
}

func IndexWalker(ix int, f NodeFunc) WalkFunc {
	return func(current, parent *yaml.Node, pos int, opts *WalkOptions) (WalkStatus, error) {
		if parent == nil || parent.Kind != yaml.SequenceNode {
			return opts.MissStatus, nil
		}
		if ix > pos {
			return WalkBreadthFirst, nil
		}
		if ix < pos {
			return WalkPrune, nil
		}
		err := f(current)
		return opts.MatchStatus(), err
	}
}

type PathMatcher interface {
	Match(node *yaml.Node, fn NodeFunc) error
}

func KeyMatcher(key string) PathMatcher {
	return keyPathMatcher(key)
}

type keyPathMatcher string

func (pm keyPathMatcher) Match(node *yaml.Node, fn NodeFunc) error {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	return Walk(node, KeyStringWalker(string(pm), fn), WithMaxDepth(0))
}

func NodeMatcher(n *yaml.Node) PathMatcher {
	return (*nodePathMatcher)(n)
}

type nodePathMatcher yaml.Node

func (pm *nodePathMatcher) Match(node *yaml.Node, fn NodeFunc) error {
	return Walk(node, func(current, parent *yaml.Node, pos int, opts *WalkOptions) (WalkStatus, error) {
		if !Equal((*yaml.Node)(pm), current) {
			return opts.MissStatus, nil
		}
		err := fn(current)
		return opts.MatchStatus(), err
	})
}

func IndexMatcher(i int) PathMatcher {
	return indexPathMatcher(i)
}

type indexPathMatcher int

func (pm indexPathMatcher) Match(node *yaml.Node, fn NodeFunc) error {
	if node.Kind != yaml.SequenceNode {
		return nil
	}
	return Walk(node, IndexWalker(int(pm), fn), WithMaxDepth(0))
}

func AnyMatcher(walkOpts ...WalkOpt) PathMatcher {
	return &anyPathMatcher{
		walkOpts: walkOpts,
	}
}

type anyPathMatcher struct {
	walkOpts []WalkOpt
}

func (pm *anyPathMatcher) Match(node *yaml.Node, fn NodeFunc) error {
	return Walk(node, func(current, parent *yaml.Node, pos int, opts *WalkOptions) (WalkStatus, error) {
		if parent != nil && parent.Kind == yaml.MappingNode {
			// match on all map keys, so send map value as next node
			err := fn(parent.Content[pos+1])
			return opts.MissStatus, err
		}
		err := fn(current)
		return opts.MissStatus, err
	}, pm.walkOpts...)
}

func WalkPathMatchers(root *yaml.Node, fn NodeFunc, matchers ...PathMatcher) error {
	matchFn := fn
	for i := len(matchers) - 1; i >= 0; i-- {
		prevFn, i := matchFn, i
		matchFn = func(node *yaml.Node) error {
			return matchers[i].Match(node, prevFn)
		}
	}
	return matchFn(unwrapDocument(root))
}

func WalkPath(root *yaml.Node, fn NodeFunc, path ...interface{}) error {
	matchers := []PathMatcher{}
	for _, p := range path {
		switch pp := p.(type) {
		case string:
			matchers = append(matchers, KeyMatcher(pp))
		case int:
			matchers = append(matchers, IndexMatcher(pp))
		case *yaml.Node:
			matchers = append(matchers, NodeMatcher(pp))
		default:
			return fmt.Errorf("Unable to make PathMatcher from type %T (%v)", p, p)
		}
	}
	return WalkPathMatchers(root, fn, matchers...)
}
