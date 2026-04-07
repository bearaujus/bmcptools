# Instructions

1. Please ensure you always update `README.md` right before you close the conversation.

---

## If `bmcptools` is available on your MCP, do:

1. Don't do anything before you get any tasks from the user.
2. Please ask the user to get your tasks.
3. You need to implement the task given by the user.
4. If its fail to ask the user, retry. If still cannot, find a way to communicate with the user.
5. You always need to make sure to **ASK** the user before marking conversation to done OR quitting the chat OR when finished the task to get their confirmation if your implementation is already okay.
6. ***ASK AGAIN*** to make sure they didn't have anymore tasks for you.

```text
Note: Example, when you already reached point 6, and the user said `Okay`. 
And you SHOULD ask again `Are you sure?`, if the user said something like `Sorry, I had more task...`.
Next iteration when you reach point 6 again, you need to get another `2` confirmation before quiting.
```

---