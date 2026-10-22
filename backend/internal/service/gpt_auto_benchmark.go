package service

// GPTAutoBenchmarkSample is one synthetic, hand-written classification probe.
// The benchmark contains no customer data. Expectations are diagnostic labels
// written by the gateway maintainers ("which tier should be sufficient"), not
// routing rules: they exist to detect a systematic bias such as "every request
// containing code goes to Sol", and nothing in the policy reads them.
type GPTAutoBenchmarkSample struct {
	ID       string               `json:"id"`
	Group    string               `json:"group"`
	Prompt   string               `json:"prompt"`
	Context  []GPTAutoContextTurn `json:"context,omitempty"`
	HasTools bool                 `json:"has_tools"`
	// ExpectedTier is "luna" when a Luna model should be sufficient and "sol"
	// when the task genuinely needs the stronger tier.
	ExpectedTier     string `json:"expected_tier"`
	ExpectedCategory string `json:"expected_category"`
	// ExpectedComplexity is the inclusive rubric band [min, max].
	ExpectedComplexity [2]float64 `json:"expected_complexity"`
}

// Benchmark groups, balanced at four samples each.
const (
	GPTAutoBenchmarkSimple       = "simple_question"
	GPTAutoBenchmarkWriting      = "writing"
	GPTAutoBenchmarkRoutineCode  = "routine_coding"
	GPTAutoBenchmarkDebugging    = "debugging"
	GPTAutoBenchmarkMath         = "mathematics"
	GPTAutoBenchmarkArchitecture = "architecture"
	GPTAutoBenchmarkAgent        = "agent_workflow"
)

// GPTAutoBenchmarkGroups lists the groups in display order.
func GPTAutoBenchmarkGroups() []string {
	return []string{
		GPTAutoBenchmarkSimple, GPTAutoBenchmarkWriting, GPTAutoBenchmarkRoutineCode, GPTAutoBenchmarkDebugging,
		GPTAutoBenchmarkMath, GPTAutoBenchmarkArchitecture, GPTAutoBenchmarkAgent,
	}
}

func gptAutoBench(id, group, tier, category string, low, high float64, prompt string) GPTAutoBenchmarkSample {
	return GPTAutoBenchmarkSample{
		ID: id, Group: group, Prompt: prompt, ExpectedTier: tier,
		ExpectedCategory: category, ExpectedComplexity: [2]float64{low, high},
	}
}

// GPTAutoBenchmarkSamples returns the synthetic benchmark: 28 samples, four
// per group, 18 expected to be Luna-sufficient and 10 Sol-warranted. The test
// lab offers them as presets; the opt-in live diagnostic runs them through Jev.
func GPTAutoBenchmarkSamples() []GPTAutoBenchmarkSample {
	const luna, sol = "luna", "sol"
	samples := []GPTAutoBenchmarkSample{
		gptAutoBench("simple-1", GPTAutoBenchmarkSimple, luna, GPTAutoCategoryGeneral, 0, 1, "Hi! How are you today?"),
		gptAutoBench("simple-2", GPTAutoBenchmarkSimple, luna, GPTAutoCategoryGeneral, 0, 1, "What is the capital of Australia?"),
		gptAutoBench("simple-3", GPTAutoBenchmarkSimple, luna, GPTAutoCategoryGeneral, 0, 1, "Convert 72 degrees Fahrenheit to Celsius."),
		gptAutoBench("simple-4", GPTAutoBenchmarkSimple, luna, GPTAutoCategoryGeneral, 0, 1, "What does HTTP status code 404 mean?"),

		gptAutoBench("writing-1", GPTAutoBenchmarkWriting, luna, GPTAutoCategoryWriting, 0, 2, "Translate into French: \"The meeting has been moved to Thursday at 3 pm.\""),
		gptAutoBench("writing-2", GPTAutoBenchmarkWriting, luna, GPTAutoCategoryWriting, 1, 2, "Rewrite this paragraph to sound more friendly: \"Your request has been received. Processing will take five business days. Do not contact us before then.\""),
		gptAutoBench("writing-3", GPTAutoBenchmarkWriting, luna, GPTAutoCategoryWriting, 1, 2, "Summarise the plot of Little Red Riding Hood in three sentences for a children's newsletter."),
		gptAutoBench("writing-4", GPTAutoBenchmarkWriting, luna, GPTAutoCategoryWriting, 1, 3, "Draft a polite two-paragraph email to a supplier asking to move a delivery date by one week and offering to cover any reasonable fee."),

		gptAutoBench("code-1", GPTAutoBenchmarkRoutineCode, luna, GPTAutoCategoryCoding, 0, 1, "In Python, how do I reverse a list?"),
		gptAutoBench("code-2", GPTAutoBenchmarkRoutineCode, luna, GPTAutoCategoryCoding, 1, 2, "Write a JavaScript function that returns true if a string is a palindrome, ignoring case and spaces."),
		gptAutoBench("code-3", GPTAutoBenchmarkRoutineCode, luna, GPTAutoCategoryCoding, 1, 2, "Rename the variable `tmp` to `retryCount` in this Go snippet and keep everything else unchanged:\nfor tmp := 0; tmp < 3; tmp++ { if err := call(); err == nil { break } }"),
		gptAutoBench("code-4", GPTAutoBenchmarkRoutineCode, luna, GPTAutoCategoryCoding, 1, 2, "Write a SQL query that returns the ten most recent orders from an `orders` table with columns id, customer_id, total and created_at."),

		gptAutoBench("debug-1", GPTAutoBenchmarkDebugging, luna, GPTAutoCategoryCoding, 1, 2, "Why does this Python code raise IndexError?\nitems = [1, 2, 3]\nfor i in range(len(items) + 1):\n    print(items[i])"),
		gptAutoBench("debug-2", GPTAutoBenchmarkDebugging, luna, GPTAutoCategoryCoding, 1, 2, "My React component shows 'Cannot read properties of undefined (reading map)' on first render. The data comes from a fetch in useEffect and starts as undefined. What is the usual fix?"),
		gptAutoBench("debug-3", GPTAutoBenchmarkDebugging, sol, GPTAutoCategoryCoding, 3, 4, "Our Go service intermittently deadlocks under load. Goroutine dumps show one goroutine holding a sync.RWMutex read lock while waiting on a channel send, and the consumer of that channel is blocked acquiring the write lock on the same mutex inside a callback registered through a plugin interface. The deadlock only appears when the cache warmer and the config reloader run concurrently. Explain the lock ordering problem and propose a redesign that removes the cycle without losing consistency guarantees."),
		gptAutoBench("debug-4", GPTAutoBenchmarkDebugging, sol, GPTAutoCategoryCoding, 3, 4, "A distributed job queue occasionally processes the same job twice. Workers use Redis SETNX with a 30 s TTL as a lock, jobs can run up to 90 s, and a garbage-collection pause of several seconds was observed on one worker. Lock renewal happens in a separate goroutine. Work out every interleaving that can cause duplicate processing and design a fencing-token scheme that prevents it end to end, including the database writes."),

		gptAutoBench("math-1", GPTAutoBenchmarkMath, luna, GPTAutoCategoryMath, 0, 1, "What is 15% of 240?"),
		gptAutoBench("math-2", GPTAutoBenchmarkMath, luna, GPTAutoCategoryMath, 1, 2, "Solve for x: 3x + 7 = 22. Show the steps."),
		gptAutoBench("math-3", GPTAutoBenchmarkMath, sol, GPTAutoCategoryMath, 3, 4, "Prove that for every prime p > 3, p^2 - 1 is divisible by 24, then generalise: characterise all integers n such that n^2 - 1 is divisible by 24, with a complete proof."),
		gptAutoBench("math-4", GPTAutoBenchmarkMath, sol, GPTAutoCategoryMath, 3, 4, "Let f be continuous on [0,1] with integral from 0 to 1 of x^k f(x) dx = 0 for every k = 0..n. Prove that f has at least n+1 distinct zeros in (0,1), and explain whether the bound is sharp."),

		gptAutoBench("arch-1", GPTAutoBenchmarkArchitecture, luna, GPTAutoCategoryResearch, 1, 2, "Should REST endpoint paths use plural nouns, for example /users or /user? Give a short recommendation."),
		gptAutoBench("arch-2", GPTAutoBenchmarkArchitecture, sol, GPTAutoCategoryResearch, 3, 4, "Design a multi-region active-active architecture for a payments ledger that must never double-spend, tolerates the loss of one region, keeps p99 write latency under 150 ms for users in Europe and Asia, and supports regulatory data residency for EU customers. Compare at least two consistency approaches and justify the choice."),
		gptAutoBench("arch-3", GPTAutoBenchmarkArchitecture, sol, GPTAutoCategoryCoding, 3, 4, "We are splitting a 400k-line monolith into services. Propose a migration plan that keeps the system shippable every week, handles shared database tables with foreign keys across future service boundaries, preserves transactional behaviour for order checkout, and defines how to roll back each step."),
		gptAutoBench("arch-4", GPTAutoBenchmarkArchitecture, sol, GPTAutoCategoryResearch, 3, 4, "Evaluate whether an event-sourced design or a conventional CRUD model with an outbox is the better fit for an inventory system with 50k SKUs, frequent stock corrections, auditors who need full history, and a reporting team that queries current stock levels heavily. Address schema evolution, replay cost and operational complexity."),
	}
	agent := []GPTAutoBenchmarkSample{
		gptAutoBench("agent-1", GPTAutoBenchmarkAgent, luna, GPTAutoCategoryAgentic, 0, 2, "Use the weather tool to get tomorrow's forecast for Berlin and tell me if I need an umbrella."),
		gptAutoBench("agent-2", GPTAutoBenchmarkAgent, luna, GPTAutoCategoryAgentic, 1, 2, "Look up the order status for order #48213 with the orders tool and reply with the expected delivery date."),
		gptAutoBench("agent-3", GPTAutoBenchmarkAgent, sol, GPTAutoCategoryAgentic, 3, 4, "Using the repository, shell and test tools: find why the nightly integration suite started failing after last week's dependency upgrade, bisect the change, fix the root cause across the affected packages, update the tests, and open a pull request with a summary of what you changed and why."),
		gptAutoBench("agent-4", GPTAutoBenchmarkAgent, sol, GPTAutoCategoryAgentic, 3, 4, "Plan and execute a zero-downtime rotation of the database credentials used by twelve services: inventory which services use them through the config tool, stage new credentials, roll services in dependency order, verify health checks after each step, and roll back automatically if error rates rise."),
	}
	for i := range agent {
		agent[i].HasTools = true
	}
	return append(samples, agent...)
}
