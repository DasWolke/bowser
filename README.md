你好！
很冒昧用这样的方式来和你沟通，如有打扰请忽略我的提交哈。我是光年实验室（gnlab.com）的HR，在招Golang开发工程师，我们是一个技术型团队，技术氛围非常好。全职和兼职都可以，不过最好是全职，工作地点杭州。
我们公司是做流量增长的，Golang负责开发SAAS平台的应用，我们做的很多应用是全新的，工作非常有挑战也很有意思，是国内很多大厂的顾问。
如果有兴趣的话加我微信：13515810775  ，也可以访问 https://gnlab.com/，联系客服转发给HR。
# Bowser

Bowser is a modern, simple, and grokable SSH daemon built to act as a bastion and SSH certificate authority. Bastion provides users with a unobtrusive yet highly secure flow to SSH. Bowser was built at [Discord](https://discordapp.com/).

### Features
- Three-Factor authentication using SSH keys, passwords, and TOTP
- Automatic generation of signed SSH keys and certificates for access to proxied servers
- Extensive logging too multiple outlets
- Simple, auditable codebase

## Usage

### Example Config

```json
{
  "bind": "0.0.0.0:22",
  "discord_webhooks": ["https://canary.discordapp.com/api/webhooks/255545515817566228/my_discord_webhook_token"]
}
```

### Example Accounts

```json
[
  {
    "username": "andrei",
    "password": "$2a$15$QWu4umMh.ZRd5RtrMNkY4e0N197Uha8poioQsEn5spjz5brU8FIRK",
    "ssh-keys": [
      "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCooBb+XKzBkDbr2qc1NM5iTRoaKXtjZPS0l9eOD+szEowHX5P+Ab4uvWcs6KUPcbITBZK60AN3Pi6mt5sTUQuqkFOGJolh6sDXpiBis7bkxQoDe11oOeHfBBHE5YfUaa7naLopN0cSXTkusY/ReNQDvIjQVjfmwoGA2pW96wV1oqnPDHz8HRUcHjfTdjovWY8xMRO0ZsHuavOdk8O+FYaD8BIO3i0bIa/tFe56Eme2FuCN77PgsHVA0HTzMAUGNpZU0zYsk8B5pjpQQyScSpE2ZfF2JqxcTl4KrnxWA3XtDtD3+lPR7ryWy+qDgrf9UxkuP7FEdIE6yD4lZdu0UdcD gopher@google.com"
    ],
    "mfa": {
      "totp": "AAAAAAAAAAAAAAAA"
    },
  }
]
```

### Example SSH Config

```
Host bastion
  Hostname bastion.my.corp
  Port 22
  ControlMaster auto
  ControlPath /tmp/ssh-control-%r@%h:%p
  ControlPersist 30m

Host credit-card-database1
  Hostname credit-card-database1.my.corp
  ProxyCommand ssh -W %h:%p bastion
```

## FAQ

### OpenSSH fails with "no private key for certificate"

This is caused by [this](https://bugzilla.mindrot.org/show_bug.cgi?id=2550) OpenSSH bug. Upgrade your version of OpenSSH to resolve.
