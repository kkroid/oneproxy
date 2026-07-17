// i18n.h — Minimal hardcoded i18n for OneProxy tray
#pragma once
#include <QString>
#include <QLocale>

struct Strings {
    QString running, stopped;
    QString start, stop, restart;
    QString check, flushDNS, quit;
    QString timeout;
    QString unifiedLabel;
    QString autoStart;
    QString systemProxy;
    QString routingMode, modeGlobal, modeRule, modeDirect;
    QString openConfig;
    QString exportConfig, importConfig;
};

inline Strings getStrings() {
    bool isChinese = QLocale::system().language() == QLocale::Chinese;
    return isChinese ? Strings{
        "🟢 运行中", "🔴 已停止",
        "启动所有代理", "停止所有代理", "重启所有代理",
        "立即检查所有节点", "立即刷新 DNS", "退出",
        "超时",
        "统一出口 %1",
        "开机自启",
        "设为系统代理",
        "代理模式", "全局", "规则", "直连",
        "打开配置文件",
        "导出配置...", "导入配置...",
    } : Strings{
        "🟢 Running", "🔴 Stopped",
        "Start All Proxies", "Stop All Proxies", "Restart All Proxies",
        "Check All Nodes", "Flush DNS", "Quit",
        "timeout",
        "Unified %1",
        "Auto-start on boot",
        "Set as system proxy",
        "Routing Mode", "Global", "Rule", "Direct",
        "Open Config File",
        "Export Config...", "Import Config...",
    };
}
