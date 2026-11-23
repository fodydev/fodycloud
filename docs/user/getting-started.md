# Getting started

In this tutorial you put a small website online with FodyCloud. You create an account, deploy an
example app from GitHub, and open it at its own HTTPS address. It takes about ten minutes.

**You need:** an email address and a web browser. You don't need to install anything.

## 1. Create your account

1. Go to [dashboard.fodycloud.example](https://dashboard.fodycloud.example) and select **Sign up**.
2. Enter your email address and a password of at least 10 characters, then select
   **Create account**.
3. Open the email titled **Confirm your FodyCloud email** and select the link in it.

You are now logged in, and the **Apps** page is empty. If the email doesn't arrive within a few
minutes, check your spam folder, or select **Resend the link** at the top of the dashboard.

## 2. Create your first app

FodyCloud deploys code from a **public** repository on GitHub, GitLab or Codeberg. For this
tutorial you use an example Node.js app.

1. On the **Apps** page, find the **New app** form.
2. Fill it in:

   | Field | Value |
   |---|---|
   | Name | `hello-` followed by your first name, for example `hello-amina` |
   | Public repository URL | `https://github.com/heroku/node-js-getting-started` |
   | Branch | `main` |

3. Select **Analyse repository**.

FodyCloud opens your app's page and shows **Analysing your repository…**. It reads the code to
find out how it is built. **Nothing is deployed yet.** The page refreshes by itself.

The name becomes part of your app's address, so it must be unique on FodyCloud: 3 to 32
lowercase letters, digits and hyphens, starting with a letter. If someone already uses it,
choose another one.

## 3. Review the stack

After a few seconds the **Review your stack** card appears. It says what FodyCloud found:

- **Detected:** Node.js · Node server (Express, Fastify…)
- **Run command:** `node index.js`, the command that starts this example's web server

The *stack* is the language and framework of your app, plus the commands that build and start
it. Here the detection is right, so you don't change anything.

Select **Confirm and deploy**.

> When the detection is wrong or finds nothing, you choose the language and framework yourself
> in the same card. See [Choose your stack](stacks.md).

## 4. Watch the deployment

The **Latest deployment** card shows the build log live. FodyCloud:

1. downloads your code;
2. installs its dependencies;
3. starts your app and checks that it keeps running.

When the log ends with **Deployment succeeded**, the status at the top of the page becomes
**running**. The first build of an app usually takes one to three minutes; later builds are
faster.

## 5. Open your app

Select the address under your app's name. It looks like
`https://hello-amina.apps.fodycloud.example`.

The example app's home page opens, served over HTTPS. Your app is online and anyone with the
address can visit it.

## What you did

- You created an account and confirmed your email.
- FodyCloud analysed a repository and you confirmed its stack.
- FodyCloud built the app, started it and gave it an HTTPS address.

## Next steps

- [Deploy your own app](deploy-an-app.md): prepare your repository and deploy it.
- [Choose your stack](stacks.md): Python, Node.js, PHP, Go, Java and static websites, and what
  to do when the detection is wrong.
- [Environment variables](environment-variables.md): give your app passwords and settings
  without putting them in your code.
