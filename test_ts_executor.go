package main

import (
	"fmt"
	"google.golang.org/adk/tool"
)

// Simple test program to verify the TypeScript executor works
func main() {
	executor := &TypeScriptExecutor{}

	// Test 1: Simple console.log
	fmt.Println("Test 1: Simple console.log")
	result1 := executor.Execute(tool.Context{}, ExecuteTypeScriptParams{
		Code: `console.log("Hello from Deno!");`,
	})
	fmt.Printf("Status: %s\n", result1.Status)
	fmt.Printf("Output: %s\n", result1.Output)
	fmt.Printf("Exit Code: %d\n\n", result1.ExitCode)

	// Test 2: Mathematical calculation
	fmt.Println("Test 2: Mathematical calculation")
	result2 := executor.Execute(tool.Context{}, ExecuteTypeScriptParams{
		Code: `
const sum = Array.from({length: 10}, (_, i) => i + 1).reduce((a, b) => a + b, 0);
console.log("Sum of 1 to 10:", sum);
`,
	})
	fmt.Printf("Status: %s\n", result2.Status)
	fmt.Printf("Output: %s\n", result2.Output)
	fmt.Printf("Exit Code: %d\n\n", result2.ExitCode)

	// Test 3: Error handling
	fmt.Println("Test 3: Error handling")
	result3 := executor.Execute(tool.Context{}, ExecuteTypeScriptParams{
		Code: `
throw new Error("This is a test error");
`,
	})
	fmt.Printf("Status: %s\n", result3.Status)
	fmt.Printf("Output: %s\n", result3.Output)
	fmt.Printf("Error Message: %s\n", result3.ErrorMessage)
	fmt.Printf("Exit Code: %d\n\n", result3.ExitCode)

	// Test 4: TypeScript features
	fmt.Println("Test 4: TypeScript features")
	result4 := executor.Execute(tool.Context{}, ExecuteTypeScriptParams{
		Code: "interface Person {\n" +
			"  name: string;\n" +
			"  age: number;\n" +
			"}\n\n" +
			"const person: Person = {\n" +
			"  name: \"Alice\",\n" +
			"  age: 30\n" +
			"};\n\n" +
			"console.log(`${person.name} is ${person.age} years old`);\n",
	})
	fmt.Printf("Status: %s\n", result4.Status)
	fmt.Printf("Output: %s\n", result4.Output)
	fmt.Printf("Exit Code: %d\n\n", result4.ExitCode)
}
