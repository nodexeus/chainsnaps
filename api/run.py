#!/usr/bin/env python
import uvicorn
from app.config import get_settings


def main():
    settings = get_settings()

    uvicorn.run(
        "app.main:app",
        host=settings.host,
        port=settings.port,
        reload=settings.reload,
        log_level=settings.log_level
    )


if __name__ == "__main__":
    main()