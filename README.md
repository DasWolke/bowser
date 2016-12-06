# Bowser
Bowser is a SSH daemon/CA built to be a more secure and auditable SSH option.

### Features

- SSH User Certificates using builtin CA
- Multi-Factor authentication using TOTP
- Extensive session logging for auditing
- Ability to record and replay all PTY sessions


# TODO
- [x] Slack/Discord webhooks
- [ ] Make sure logging covers all flows
- [ ] Add support for proper time-encoded recording of PTYs
- [ ] Add support for GPG encrypting PTY recordings
- [ ] Add support for backing up recordings to GCS/S3
- [ ] Simple telnet server for auditing sessions
- [ ] CLI for replaying sessions
