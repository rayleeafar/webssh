# Product Draft: WebSSH Manager

**Version:** 1.0 (Draft)  
**Date:** March 2026  
**Product Name:** WebSSH Manager  
**Tagline:** "SSH + SFTP in your browser — anywhere, secure, lightweight"

## 1. Overview

WebSSH Manager is a **self-hosted, zero-dependency**, lightweight web application that turns any modern browser into a full-featured SSH terminal and SFTP file manager for your remote servers.

Users can:
- Manage unlimited remote nodes (servers)
- Open instant SSH terminals
- Browse, upload, download, edit files via SFTP
- Access everything from any device with a browser

**Core promise:**  
- Only HTTPS + TLS communication  
- No browser plugins  
- No heavy dependencies  
- Runs on a single binary (or tiny Docker image)

**Target users:** Developers, DevOps engineers, system administrators, homelab enthusiasts who want a clean, private, browser-only SSH portal without installing anything on their machines.

## 2. Key Requirements (User-Provided + Perfected)

- **Backend:** Go (Golang) – chosen for performance, small binary, excellent SSH libraries
- **Frontend:** Next.js 15 (App Router + React Server Components + TypeScript)
- **Database:** SQLite (embedded, zero-config, single file)
- **Communication:** 100% HTTPS + TLS only (HTTP redirect + HSTS enforced)
- **Core Modules:**
  - SSH Node Management
  - Real-time Web Terminal
  - SFTP File Manager
