import asyncio
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from aiohttp.test_utils import TestServer, TestClient
from server.server import create_app, manager

async def run_all_tests():
    app = create_app()
    server = TestServer(app)
    client = TestClient(server)
    await client.start_server()

    try:
        # 1. Test Health Endpoint
        resp = await client.get("/health")
        assert resp.status == 200
        health_data = await resp.json()
        assert health_data["status"] == "healthy"
        print("[OK] Health endpoint verified")

        # 2. Connect Client A & Client B to room 'test-room-42'
        ws_a = await client.ws_connect("/ws/test-room-42")
        ws_b = await client.ws_connect("/ws/test-room-42")

        assert "test-room-42" in manager.rooms
        assert len(manager.rooms["test-room-42"]) == 2
        print("[OK] Both clients connected and joined room")

        # 3. Test Pair Key Exchange
        pair_msg = {"type": "pair", "payload": "X25519_PUBKEY_CLIENT_A", "nonce": "nonce_pair_a"}
        await ws_a.send_json(pair_msg)
        msg_b = await ws_b.receive_json()
        assert msg_b["type"] == "pair"
        assert msg_b["payload"] == "X25519_PUBKEY_CLIENT_A"
        print("[OK] Pair message relayed successfully")

        # 4. Test Encrypted Clipboard Transfer
        clip_msg = {"type": "clipboard", "payload": "ENCRYPTED_BASE64_CLIPBOARD", "nonce": "nonce_clip_1"}
        await ws_a.send_json(clip_msg)
        msg_b2 = await ws_b.receive_json()
        assert msg_b2["type"] == "clipboard"
        assert msg_b2["payload"] == "ENCRYPTED_BASE64_CLIPBOARD"
        print("[OK] Clipboard message relayed successfully")

        # 5. Test File Sharing (file_start, file_chunk, file_end)
        for ftype in ["file_start", "file_chunk", "file_end"]:
            fmsg = {"type": ftype, "payload": f"ENCRYPTED_{ftype.upper()}", "nonce": f"nonce_{ftype}"}
            await ws_b.send_json(fmsg)
            msg_a = await ws_a.receive_json()
            assert msg_a["type"] == ftype
            assert msg_a["payload"] == f"ENCRYPTED_{ftype.upper()}"
        print("[OK] File sharing messages relayed successfully")

        # 6. Test Screenshot Metadata
        screen_msg = {"type": "screenshot_meta", "payload": "ENCRYPTED_SCREENSHOT_META", "nonce": "nonce_screen"}
        await ws_b.send_json(screen_msg)
        msg_a2 = await ws_a.receive_json()
        assert msg_a2["type"] == "screenshot_meta"
        print("[OK] Screenshot metadata relayed successfully")

        # 7. Test Ping / Pong
        ping_msg = {"type": "ping", "payload": "", "nonce": "nonce_ping"}
        await ws_a.send_json(ping_msg)
        pong_resp = await ws_a.receive_json()
        assert pong_resp["type"] == "pong"
        print("[OK] Ping / Pong verified")

        # 8. Test Disconnection & Room Cleanup
        await ws_a.close()
        await ws_b.close()
        await asyncio.sleep(0.1)
        assert "test-room-42" not in manager.rooms
        print("[OK] Room auto-cleanup upon disconnect verified")

        print("\n===============================")
        print("ALL RELAY SERVER TESTS PASSED!")
        print("===============================")
    finally:
        await client.close()

if __name__ == "__main__":
    asyncio.run(run_all_tests())
