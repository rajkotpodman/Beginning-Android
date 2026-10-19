"""
Beginning Android - Lightweight End-to-End Encrypted Relay Server
Zero-knowledge blind WebSocket relay connecting paired Android and PC devices.
"""

import asyncio
import json
import logging
import os
import ssl
import time
from typing import Dict, Set
from aiohttp import web, WSMsgType

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger("relay")

PORT = int(os.getenv("PORT", "8080"))
HOST = os.getenv("HOST", "0.0.0.0")
TLS_CERT = os.getenv("TLS_CERT", "")
TLS_KEY = os.getenv("TLS_KEY", "")
STALE_TIMEOUT = int(os.getenv("STALE_TIMEOUT", "300"))  # seconds

VALID_TYPES = {
    "key_exchange",
    "pair",
    "clipboard",
    "voice_stream",
    "screenshot",
    "screenshot_meta",
    "screenshot_start",
    "file_start",
    "file_chunk",
    "file_end",
    "ping",
    "pong",
}

class RoomManager:
    def __init__(self):
        self.rooms: Dict[str, Set[web.WebSocketResponse]] = {}
        self.last_active: Dict[str, float] = {}
        self.ws_to_room: Dict[web.WebSocketResponse, str] = {}

    def join(self, room_id: str, ws: web.WebSocketResponse):
        if ws in self.ws_to_room:
            self.leave(ws)
        if room_id not in self.rooms:
            self.rooms[room_id] = set()
        self.rooms[room_id].add(ws)
        self.ws_to_room[ws] = room_id
        self.last_active[room_id] = time.time()
        logger.info(f"Client joined room '{room_id}' (peers: {len(self.rooms[room_id])})")

    def leave(self, ws: web.WebSocketResponse):
        room_id = self.ws_to_room.pop(ws, None)
        if room_id and room_id in self.rooms:
            self.rooms[room_id].discard(ws)
            logger.info(f"Client left room '{room_id}' (remaining: {len(self.rooms[room_id])})")
            if not self.rooms[room_id]:
                del self.rooms[room_id]
                self.last_active.pop(room_id, None)
                logger.info(f"Room '{room_id}' closed")

    async def broadcast(self, sender_ws: web.WebSocketResponse, message_str: str):
        room_id = self.ws_to_room.get(sender_ws)
        if not room_id or room_id not in self.rooms:
            return
        self.last_active[room_id] = time.time()
        peers = [ws for ws in self.rooms[room_id] if ws != sender_ws and not ws.closed]
        for peer in peers:
            try:
                await peer.send_str(message_str)
            except Exception as e:
                logger.warning(f"Forward failed in room '{room_id}': {e}")

    def cleanup_stale_rooms(self):
        now = time.time()
        stale = [
            r for r, last in self.last_active.items()
            if now - last > STALE_TIMEOUT and (r not in self.rooms or not self.rooms[r])
        ]
        for r in stale:
            self.rooms.pop(r, None)
            self.last_active.pop(r, None)
            logger.info(f"Cleaned up stale room '{r}'")

manager = RoomManager()

async def websocket_handler(request: web.Request) -> web.WebSocketResponse:
    ws = web.WebSocketResponse(heartbeat=30.0)
    await ws.prepare(request)

    room_param = request.match_info.get("room") or request.query.get("room")
    if room_param:
        manager.join(room_param, ws)

    try:
        async for msg in ws:
            if msg.type == WSMsgType.TEXT:
                try:
                    data = json.loads(msg.data)
                except json.JSONDecodeError:
                    continue

                msg_type = data.get("type")
                if msg_type not in VALID_TYPES:
                    continue

                if msg_type == "pair":
                    room_key = data.get("room")
                    if room_key and manager.ws_to_room.get(ws) != str(room_key):
                        manager.join(str(room_key), ws)
                    await manager.broadcast(ws, msg.data)
                    continue

                if msg_type == "ping":
                    await ws.send_json({"type": "pong", "time": int(time.time() * 1000)})
                    await manager.broadcast(ws, msg.data)
                    continue

                await manager.broadcast(ws, msg.data)

            elif msg.type == WSMsgType.ERROR:
                logger.warning(f"WebSocket error: {ws.exception()}")
    finally:
        manager.leave(ws)

    return ws

async def health_check(_: web.Request) -> web.Response:
    return web.json_response({
        "status": "healthy",
        "active_rooms": len(manager.rooms),
        "active_connections": len(manager.ws_to_room),
    })

async def stale_cleanup_task(app: web.Application):
    try:
        while True:
            await asyncio.sleep(60)
            manager.cleanup_stale_rooms()
    except asyncio.CancelledError:
        pass

async def start_background_tasks(app: web.Application):
    app["cleanup_task"] = asyncio.create_task(stale_cleanup_task(app))

async def cleanup_background_tasks(app: web.Application):
    app["cleanup_task"].cancel()
    await app["cleanup_task"]

def create_app() -> web.Application:
    app = web.Application()
    app.router.add_get("/", health_check)
    app.router.add_get("/health", health_check)
    app.router.add_get("/ws", websocket_handler)
    app.router.add_get("/ws/{room}", websocket_handler)
    app.on_startup.append(start_background_tasks)
    app.on_cleanup.append(cleanup_background_tasks)
    return app

def main():
    ssl_context = None
    if TLS_CERT and TLS_KEY and os.path.exists(TLS_CERT) and os.path.exists(TLS_KEY):
        ssl_context = ssl.create_default_context(ssl.Purpose.CLIENT_AUTH)
        ssl_context.load_cert_chain(certfile=TLS_CERT, keyfile=TLS_KEY)
        logger.info(f"TLS enabled using cert: {TLS_CERT}")

    app = create_app()
    logger.info(f"Starting Beginning Android Relay Server on {HOST}:{PORT}")
    web.run_app(app, host=HOST, port=PORT, ssl_context=ssl_context)

if __name__ == "__main__":
    main()
