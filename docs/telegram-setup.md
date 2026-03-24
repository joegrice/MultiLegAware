# Telegram Bot Setup Guide

This guide walks through everything needed to create a Telegram bot, find your
chat ID, and verify the integration before running MultiLegAware.

---

## 1. Creating a Bot with @BotFather

1. Open Telegram (desktop or mobile).
2. Search for **@BotFather** and open the chat. Verify the blue checkmark —
   this is the official Telegram bot management account.
3. Send the command:
   ```
   /newbot
   ```
4. BotFather will ask for a **display name** (e.g. `My Commute Bot`). This is
   shown at the top of the chat. It can contain spaces.
5. BotFather will then ask for a **username**. This must:
   - end in `bot` (e.g. `mycommute_bot` or `MyCommuteBot`)
   - be globally unique across all Telegram bots
   - contain only letters, numbers, and underscores
6. On success, BotFather replies with your token:
   ```
   Done! Congratulations on your new bot. You will find it at t.me/mycommute_bot.
   Use this token to access the HTTP API:

   123456789:ABCdefGHIjklMNOpqrSTUvwxYZ-1234567
   ```
   The token format is always `{numeric_id}:{alphanumeric_string}`.
7. Copy the token and set it as `TELEGRAM_BOT_TOKEN` in your `.env` file:
   ```
   TELEGRAM_BOT_TOKEN=123456789:ABCdefGHIjklMNOpqrSTUvwxYZ-1234567
   ```

> **Keep this token secret.** Anyone with the token can send messages as your
> bot. Do not commit it to version control. Do not log it.

---

## 2. Getting Your Chat ID

The bot needs to know which chat to send messages to. Follow these steps:

1. Open Telegram and find your newly created bot (search for its username).
2. Send **any message** to the bot — it must receive at least one message before
   `getUpdates` will return results. Sending `/start` works well.
3. In a browser or `curl`, call the `getUpdates` endpoint (replace `TOKEN` with
   your actual token):
   ```
   https://api.telegram.org/botTOKEN/getUpdates
   ```
   Example:
   ```bash
   curl -s "https://api.telegram.org/bot123456789:ABCdef.../getUpdates" | jq .
   ```
4. Find the `result` array in the response. Look for the `message.chat` object:
   ```json
   {
     "result": [
       {
         "message": {
           "chat": {
             "id": 987654321,
             "first_name": "Your Name",
             "type": "private"
           },
           "text": "/start"
         }
       }
     ]
   }
   ```
   The value of `result[0].message.chat.id` is your chat ID.
5. Set it in your `.env` file:
   ```
   TELEGRAM_CHAT_ID=987654321
   ```

> **Group chats:** If you added the bot to a group, the chat `id` will be a
> **negative** integer (e.g. `-1001234567890`). Negative IDs are valid — use
> the value exactly as returned, including the minus sign.

> **If `getUpdates` returns an empty array:** You haven't sent the bot a message
> yet. Go back to Telegram, open the bot chat, and send `/start`. Then call
> `getUpdates` again.

---

## 3. Testing the Bot Manually

Before starting the service, verify the bot can send messages by calling the
Telegram API directly with `curl`:

```bash
curl -s -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
  -H "Content-Type: application/json" \
  -d "{
    \"chat_id\": \"${TELEGRAM_CHAT_ID}\",
    \"text\": \"Hello from MultiLegAware \\\\.\",
    \"parse_mode\": \"MarkdownV2\"
  }"
```

A successful response looks like:

```json
{
  "ok": true,
  "result": {
    "message_id": 42,
    "chat": {
      "id": 987654321,
      "type": "private"
    },
    "text": "Hello from MultiLegAware .",
    "date": 1700000000
  }
}
```

If `ok` is `true` and you see the message in Telegram, your bot token and chat
ID are correct.

---

## 4. Troubleshooting Telegram

### Bot not receiving messages / getUpdates is empty

You must send at least one message **to the bot** before `getUpdates` will
return any results. Open the bot chat in Telegram and send `/start`. If you
previously blocked the bot, unblock it first.

### sendMessage returns 400 Bad Request

```json
{"ok": false, "error_code": 400, "description": "Bad Request: can't parse entities"}
```

This is a **MarkdownV2 escaping error**. A character in the message text has
special meaning in MarkdownV2 and wasn't escaped. The characters that must be
escaped with a backslash are:

```
_  *  [  ]  (  )  ~  `  >  #  +  -  =  |  {  }  .  !
```

When testing manually with `curl`, note that shell escaping compounds with
JSON escaping — the backslash in `\\.` above is a shell-escaped `\.` (escaped
dot for MarkdownV2). If in doubt, use `parse_mode: ""` (omit it) for plain-text
testing.

### sendMessage returns 401 Unauthorized

```json
{"ok": false, "error_code": 401, "description": "Unauthorized"}
```

The bot token is wrong or has been revoked. To regenerate the token:

1. Open @BotFather.
2. Send `/mybots` and select your bot.
3. Select **API Token** → **Revoke current token**.
4. Copy the new token and update `TELEGRAM_BOT_TOKEN`.

### sendMessage returns 403 Forbidden

The bot was blocked by the user, or the bot was kicked from a group. In a
private chat: go to the bot in Telegram, tap **Unblock**. In a group: re-add
the bot with admin rights if needed.

### sendMessage returns 400: chat not found

The `TELEGRAM_CHAT_ID` is wrong. Re-run the `getUpdates` step to get the
correct ID. Ensure you are using the integer value exactly (with the minus sign
for group chats).
