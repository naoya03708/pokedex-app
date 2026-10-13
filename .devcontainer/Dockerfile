FROM golang:1.27

ARG USERNAME=dev
ARG UID=1000
ARG GID=1000

# GitHub CLI, from the official apt repository.
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates git python3 \
    && mkdir -p -m 755 /etc/apt/keyrings \
    && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg -o /etc/apt/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list \
    && apt-get update && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/*

# A non-root user. The directories that devcontainer.json mounts volumes on
# are created here, owned by that user: Docker copies the owner of an image
# directory into a new volume, so the volumes are writable without root.
RUN groupadd --gid ${GID} ${USERNAME} \
    && useradd --uid ${UID} --gid ${GID} --create-home --shell /bin/bash ${USERNAME} \
    && mkdir -p /home/${USERNAME}/.config/gh /home/${USERNAME}/.claude-config /home/${USERNAME}/.cache/go-build \
    && chown -R ${UID}:${GID} /home/${USERNAME}

USER ${USERNAME}
ENV HOME=/home/${USERNAME}

# Claude Code, with the official native installer (into ~/.local/bin). Its
# state (login, settings, history) lives in CLAUDE_CONFIG_DIR, which
# devcontainer.json mounts as a volume.
RUN curl -fsSL https://claude.ai/install.sh | bash
ENV PATH="/home/${USERNAME}/.local/bin:${PATH}"
ENV CLAUDE_CONFIG_DIR=/home/${USERNAME}/.claude-config
ENV GOCACHE=/home/${USERNAME}/.cache/go-build
