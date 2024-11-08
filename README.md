# TrojanBotProxy

Minimal example for working with [TrojanBot Telegram bot](https://x.com/TrojanOnSolana) via code. This example only  allows you to buy tokens via TrojanBot by pasting token addresses to the console.

## Build and run

If you want to build it from source, download Go toolchain and write

```console
go build main.go
```

in the console, then run executable.

If you want to run it from source, download Go toolchain and write

```console
go run main.go
```

in the console.

## Configuration

`config.toml` is provided to setup application id and application hash that are obtainable from [this link](https://my.telegram.org/apps). In addition to that, phone number and TrojanBot telegram contact name is also required.

## Quick start

Program will load `config.toml` file and then try to log in to Telegram account. User will be asked for login code and 2FA code (if setup for account). After successful authentication, user will be repeatadly asked for a token address to buy by bot. Inserting `quit` or `exit` will disconnect session and terminate program.

Application will always use the first button on the third row from the bot message, if so many buttons are found

![picrel](assets/img/image.png)

## Sample output

```console
Enter code: 45956
Enter token address: CrUJuZPfmE9QSXY916c2AWw3mQtzmbLijADJCJHdAxrc
2024/11/08 11:52:39 INFO buy order placed successfully button="✅ 0.002 SOL"
2024/11/08 11:52:46 INFO reply message message="Buy $TRUMP401K — (Trump401K) 📈 • 🫧\nCrUJuZPfmE9QSXY916c2AWw3mQtzmbL
ijADJCJHdAxrc\nShare token with your Reflink\n\nBalance: 0.003 SOL — W1 ✏️\nPrice: $0.0519 — LIQ: $205.8K — MC: $518.98M\n5m: 0% — 1h: 0%\nRenounced ✅\n\n\n🔴 Insufficient balance for buy amount + gas"
Enter token address: exit
2024/11/08 11:53:03 INFO exiting...
2024/11/08 11:53:03 INFO telegram client finished
```

## Use case

This repo serves as a base for being a proxy to TrojanBot Telegram bot. Assuming you have a good source of tokens to buy, this application just speed up a process to paste their addresses into TrojanBot, comparing to using TrojanBot manually.

Be aware that this repo is only minimal example and should be treated as proof of concept, not finished product. Additionally you still need to manually set up TrojanBot like autosell conditions, setup wallets etc.
