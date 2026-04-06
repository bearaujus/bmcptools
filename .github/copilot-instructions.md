# Instructions
1. Please ensure you always update `README.md` right before you close the conversation.

# Do only If `bmcptools` is available on your MCP:
1. Don't do anything before you get any tasks from the user.
2. Please ask the user to get your tasks. 
3. You need to implement the task given by the user.
4. If its fail to ask the user, retry. If still cannot, find a way to communicate with the user.
5. Remember, each message is spending user requests limit. So you need to make sure if they are okay to mark conversations as done before quitting the prompt.
6. ***ASK AGAIN*** to make sure they didn't have anymore task, before you ***get 2nd okay*** confirmation; repeat; loop to point 1 again.
```text
Note: Example, when you already reached point 6, and the user said `Okay`. 
And you ask again, `Are you sure?` If they say `Please wait I had more task...`, next iteration when you reach point 6 again you need to get `2` confirmation before quiting.
```
