---
paths:
  - 'backend/**/*.go'
---

# Backend — SOLID (pragmatic, not strict)

Apply these to keep the backend maintainable. Use judgement — don't over-engineer simple cases, don't ignore them when they matter.

## Single Responsibility

- Each struct/service should have one clear reason to change
- If a service is doing unrelated things (e.g. scanning AND analytics), split it

## Open/Closed

- Extend behaviour via interfaces and new implementations, not by modifying existing working code
- New ATS providers, LLM backends, etc. should be addable without touching existing ones

## Liskov Substitution

- Implementations of an interface must be fully interchangeable
- Don't add methods to a concrete type that callers depend on — put them in the interface

## Interface Segregation

- Keep interfaces small and focused
- Don't force a type to implement methods it doesn't need
- Prefer multiple small interfaces over one large one

## Dependency Inversion

- High-level components (workers, handlers) depend on abstractions (interfaces)
- Low-level components (stores, HTTP clients) implement those abstractions
- This is already enforced by the service layer rule — keep it that way

## Practical reminders

- Prefer composition over inheritance (Go doesn't have inheritance anyway — use embedding sparingly)
- A function that does one thing and does it well beats a flexible one that does three things poorly
- New behaviour via new types/functions, not by adding optional parameters to existing ones
