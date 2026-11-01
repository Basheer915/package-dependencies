package main

import "fmt"

type StackFrame struct {
	PackageName  string
	Depth        int
	NextChild    int
	Dependencies []string
}

func buildDependencyGraph(
	root string,
	maxDepth int,
	getDependencies func(string) ([]string, error),
) (map[string][]string, []string, error) {

	graph := make(map[string][]string)
	expandedDepth := make(map[string]int)
	visiting := make(map[string]bool)
	var cycles []string

	graph[root] = []string{}

	var rootDependencies []string

	if maxDepth > 0 {
		var err error
		rootDependencies, err = getDependencies(root)
		if err != nil {
			return nil, nil, err
		}
	}

	stack := []StackFrame{
		{
			PackageName:  root,
			Depth:        0,
			Dependencies: rootDependencies,
		},
	}

	visiting[root] = true

	for len(stack) > 0 {
		top := &stack[len(stack)-1]

		if top.NextChild >= len(top.Dependencies) {
			if top.Depth < maxDepth {
				previousDepth, exists := expandedDepth[top.PackageName]
				if !exists || top.Depth < previousDepth {
					expandedDepth[top.PackageName] = top.Depth
				}
			}

			delete(visiting, top.PackageName)
			stack = stack[:len(stack)-1]
			continue
		}

		dependency := top.Dependencies[top.NextChild]
		top.NextChild++

		graph[top.PackageName] = append(
			graph[top.PackageName],
			dependency,
		)

		if visiting[dependency] {
			cycles = append(cycles, top.PackageName+" -> "+dependency)
			continue
		}

		depth := top.Depth + 1
		previousDepth, exists := expandedDepth[dependency]
		if exists && previousDepth <= depth {
			continue
		}

		graph[dependency] = []string{}

		var dependencies []string
		if depth < maxDepth {
			var err error
			dependencies, err = getDependencies(dependency)
			if err != nil {
				return nil, nil, err
			}
		}

		visiting[dependency] = true

		stack = append(stack, StackFrame{PackageName: dependency, Depth: depth, Dependencies: dependencies})
	}

	return graph, cycles, nil
}

func printDependencyTree(root string, graph map[string][]string) {
	fmt.Println(root)

	path := make(map[string]bool)
	path[root] = true

	var printBranch func(packageName, prefix string)

	printBranch = func(packageName, prefix string) {
		dependencies := graph[packageName]

		for i, dependency := range dependencies {
			isLast := i == len(dependencies)-1

			branch := "├── "
			nextPrefix := prefix + "│   "

			if isLast {
				branch = "└── "
				nextPrefix = prefix + "    "
			}

			fmt.Print(prefix + branch + dependency)

			if path[dependency] {
				fmt.Println(" (cycle)")
				continue
			}

			fmt.Println()

			path[dependency] = true
			printBranch(dependency, nextPrefix)
			delete(path, dependency)
		}
	}

	printBranch(root, "")
}

func buildInstallationOrder(graph map[string][]string) ([]string, error) {
	var order []string

	visited := make(map[string]bool)
	visiting := make(map[string]bool)

	type Frame struct {
		PackageName string
		NextChild   int
	}

	for root := range graph {
		if visited[root] {
			continue
		}

		stack := []Frame{{PackageName: root}}
		visiting[root] = true

		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			dependencies := graph[top.PackageName]

			if top.NextChild < len(dependencies) {
				dependency := dependencies[top.NextChild]
				top.NextChild++

				if visiting[dependency] {
					return nil, fmt.Errorf("cycle detected involving package %s", dependency)
				}
				if visited[dependency] {
					continue
				}

				visiting[dependency] = true
				stack = append(stack, Frame{PackageName: dependency})
				continue
			}

			order = append(order, top.PackageName)
			visited[top.PackageName] = true
			delete(visiting, top.PackageName)

			stack = stack[:len(stack)-1]
		}
	}

	return order, nil
}
