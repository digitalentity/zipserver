/**
 * Custom Comments Integration - W3C Web Annotation Compliant Engine
 * 
 * Uses mark.js (bundled with mdbook) for range highlighting, and implements
 * an in-page sliding sidebar for multi-user threads, replies, resolve state,
 * and deletion permissions.
 */
(function() {
    'use strict';

    // ==========================================
    // 1. Configuration & Initial State
    // ==========================================
    const CURRENT_PAGE = window.location.pathname;
    
    let annotationsList = []; // Holds all annotations for the current page
    let activeSelection = null; // Holds the current selected range anchor data

    // UI references
    let popoverElement = null;
    let sidebarElement = null;
    let currentOpenThreadId = null;

    // Autocomplete references & state
    let currentUser = null;
    let autocompleteDropdown = null;
    let activeDropdownIndex = 0;
    let activeAutocompleteTextarea = null;
    let activeMentionInfo = null;

    class CommentsAPI {

        static async getAnnotations() {
            try {
                const res = await fetch(`/_/api/v1/comments?page=${encodeURIComponent(CURRENT_PAGE)}`);
                return res.ok ? res.json() : [];
            } catch (e) {
                console.error('[Comments] Failed to fetch annotations:', e);
                return [];
            }
        }

        static async saveAnnotation(annotation) {
            try {
                const res = await fetch('/_/api/v1/comments', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(annotation)
                });
                return res.ok ? res.json() : null;
            } catch (e) {
                console.error('[Comments] Failed to save annotation:', e);
                return null;
            }
        }

        static async resolveThread(annotationId, resolved) {
            try {
                const res = await fetch(`/_/api/v1/comments/${annotationId}`, {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ resolved })
                });
                return res.ok ? res.json() : null;
            } catch (e) {
                console.error('[Comments] Failed to resolve thread:', e);
                return null;
            }
        }

        static async deleteAnnotation(annotationId) {
            try {
                const res = await fetch(`/_/api/v1/comments/${annotationId}`, {
                    method: 'DELETE'
                });
                return res.status === 204;
            } catch (e) {
                console.error('[Comments] Failed to delete annotation:', e);
                return false;
            }
        }
    }

    // ==========================================
    // 3. Anchor & Selection Engine
    // ==========================================
    function hashString(str) {
        let hash = 5381;
        const clean = str.trim().replace(/\s+/g, ' ');
        for (let i = 0; i < clean.length; i++) {
            hash = ((hash << 5) + hash) + clean.charCodeAt(i);
        }
        return (hash >>> 0).toString(16);
    }

    function getCleanText(el) {
        if (!el) return '';
        const clone = el.cloneNode(true);
        const indicators = clone.querySelectorAll('.comment-paragraph-indicator');
        indicators.forEach(ind => ind.remove());
        return clone.textContent;
    }

    function getBlockElements() {
        const rawBlocks = document.querySelectorAll(
            '#mdbook-content main > p, ' +
            '#mdbook-content main > li, ' +
            '#mdbook-content main > h1, ' +
            '#mdbook-content main > h2, ' +
            '#mdbook-content main > h3, ' +
            '#mdbook-content main > h4, ' +
            '#mdbook-content main > h5, ' +
            '#mdbook-content main > h6, ' +
            '#mdbook-content main > pre, ' +
            '#mdbook-content main > blockquote'
        );
        return Array.from(rawBlocks).filter(el => {
            const isMermaid = el.classList.contains('mermaid') || 
                             el.querySelector('.mermaid') || 
                             el.tagName === 'svg' || 
                             el.classList.contains('mermaid-svg') ||
                             el.getAttribute('data-processed') === 'true';
            return !isMermaid;
        });
    }

    function getBlockIndex(el) {
        const blocks = getBlockElements();
        return Array.from(blocks).indexOf(el);
    }

    function findBlockElement(blockIdx, hash, tagName) {
        const blocks = getBlockElements();
        const targetTag = (tagName || '').toUpperCase();

        // 1. Match by index and hash
        if (blockIdx >= 0 && blockIdx < blocks.length) {
            const el = blocks[blockIdx];
            if (hashString(getCleanText(el)) === hash) {
                return el;
            }
        }
        // 2. Hash scan fallback
        for (let el of blocks) {
            if (hashString(getCleanText(el)) === hash) {
                return el;
            }
        }
        // 3. Match by index and tagName
        if (blockIdx >= 0 && blockIdx < blocks.length) {
            const el = blocks[blockIdx];
            if (el.tagName.toUpperCase() === targetTag) {
                return el;
            }
        }
        // 4. Neighborhood scan fallback
        const maxOffset = 5;
        for (let offset = 1; offset <= maxOffset; offset++) {
            const upperIdx = blockIdx + offset;
            if (upperIdx < blocks.length) {
                const el = blocks[upperIdx];
                if (el.tagName.toUpperCase() === targetTag) {
                    return el;
                }
            }
            const lowerIdx = blockIdx - offset;
            if (lowerIdx >= 0) {
                const el = blocks[lowerIdx];
                if (el.tagName.toUpperCase() === targetTag) {
                    return el;
                }
            }
        }
        // 5. Global scan fallback for tagName
        for (let el of blocks) {
            if (el.tagName.toUpperCase() === targetTag) {
                return el;
            }
        }
        return null;
    }

    function getSelectionOffsets(block, range) {
        let startOffset = 0;
        let endOffset = 0;
        let currentLength = 0;
        let foundStart = false;
        let foundEnd = false;

        const walker = document.createTreeWalker(block, NodeFilter.SHOW_TEXT, null, false);
        let node;
        while (node = walker.nextNode()) {
            const nodeLen = node.length;
            if (!foundStart && node === range.startContainer) {
                startOffset = currentLength + range.startOffset;
                foundStart = true;
            }
            if (!foundEnd && node === range.endContainer) {
                endOffset = currentLength + range.endOffset;
                foundEnd = true;
            }
            currentLength += nodeLen;
            if (foundStart && foundEnd) break;
        }
        return { start: startOffset, end: endOffset };
    }

    // ==========================================
    // 4. In-page Highlighting Loader
    // ==========================================
    function clearPageHighlights() {
        // 1. Clean up Mark.js marks
        const marks = document.querySelectorAll('mark.comment-highlight');
        marks.forEach(mark => {
            const parent = mark.parentNode;
            if (parent) {
                parent.replaceChild(document.createTextNode(mark.textContent), mark);
                parent.normalize();
            }
        });

        // 2. Clean up section-level highlights
        const sections = document.querySelectorAll('.comment-section-highlight');
        sections.forEach(sec => {
            sec.classList.remove('comment-section-highlight', 'comment-highlight', 'resolved-highlight');
            delete sec.dataset.threadId;
        });
    }

    function applyPageHighlights() {
        clearPageHighlights();
        
        // Find parent highlights (those targeting the document directly)
        const parentAnnos = annotationsList.filter(a => a.target && typeof a.target === 'object');

        parentAnnos.forEach(anno => {
            const target = anno.target;
            const selectors = target.selector || [];
            
            const quoteSel = selectors.find(s => s.type === 'TextQuoteSelector');
            const posSel = selectors.find(s => s.type === 'TextPositionSelector');
            const fragSel = selectors.find(s => s.type === 'FragmentSelector');

            if (!fragSel) return;

            // Parse Fragment values
            const params = new URLSearchParams(fragSel.value);
            const blockIdx = parseInt(params.get('blockIdx'), 10);
            const hash = params.get('blockHash');
            const tagName = params.get('tagName');

            const blockElement = findBlockElement(blockIdx, hash, tagName);
            if (!blockElement) return;

            if (posSel) {
                const start = posSel.start;
                const length = posSel.end - posSel.start;

                if (typeof window.Mark === 'function') {
                    const marker = new window.Mark(blockElement);
                    const resolvedClass = anno.resolved ? 'comment-highlight resolved-highlight' : 'comment-highlight';
                    
                    marker.markRanges([{ start, length }], {
                        className: resolvedClass,
                        each: (node) => {
                            node.dataset.threadId = anno.id;
                        }
                    });
                }
            } else {
                // Section-level annotation: highlight the block element directly
                const resolvedClass = anno.resolved ? 'comment-section-highlight comment-highlight resolved-highlight' : 'comment-section-highlight comment-highlight';
                blockElement.classList.add(...resolvedClass.split(' '));
                blockElement.dataset.threadId = anno.id;
            }
        });
    }

    // ==========================================
    // 5. Selection Listener & Popover UI
    // ==========================================
    function initSelectionTracker() {
        const main = document.querySelector('#mdbook-content main');
        if (!main) return;

        // Popover DOM Element
        popoverElement = document.createElement('div');
        popoverElement.className = 'comment-popover';
        popoverElement.innerHTML = 'Add Comment';
        popoverElement.style.display = 'none';
        document.body.appendChild(popoverElement);

        popoverElement.addEventListener('click', (e) => {
            e.stopPropagation();
            if (activeSelection) {
                openSidebar();
                openNewThreadBox(activeSelection);
                popoverElement.style.display = 'none';
                window.getSelection().removeAllRanges();
            }
        });

        document.addEventListener('mouseup', () => {
            setTimeout(handleTextSelection, 10);
        });

        document.addEventListener('mousedown', (e) => {
            if (popoverElement && !popoverElement.contains(e.target)) {
                popoverElement.style.display = 'none';
            }
        });
    }

    function initHighlightClickListener() {
        const main = document.querySelector('#mdbook-content main');
        if (!main) return;
        main.addEventListener('click', (e) => {
            const highlight = e.target.closest('mark.comment-highlight, .comment-section-highlight');
            if (highlight) {
                if (e.target.closest('a, button, input, textarea, .comment-paragraph-indicator')) {
                    return;
                }
                const threadId = highlight.dataset.threadId;
                if (threadId) {
                    e.stopPropagation();
                    openSidebar();
                    focusThread(threadId);
                }
            }
        });
    }


    function handleTextSelection() {
        const selection = window.getSelection();
        if (!selection || selection.isCollapsed) {
            activeSelection = null;
            return;
        }

        const range = selection.getRangeAt(0);
        const main = document.querySelector('#mdbook-content main');
        
        // Ensure range is inside the main block
        if (!main.contains(range.commonAncestorContainer)) {
            activeSelection = null;
            return;
        }

        // Find parent block element
        let parentBlock = range.commonAncestorContainer;
        if (parentBlock.nodeType === Node.TEXT_NODE) {
            parentBlock = parentBlock.parentNode;
        }
        
        const validTags = ['P', 'LI', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'PRE', 'BLOCKQUOTE'];
        while (parentBlock && !validTags.includes(parentBlock.tagName) && parentBlock !== main) {
            parentBlock = parentBlock.parentNode;
        }

        if (parentBlock === main || !parentBlock || parentBlock.classList.contains('mermaid') || parentBlock.querySelector('.mermaid')) {
            activeSelection = null;
            return;
        }

        const offsets = getSelectionOffsets(parentBlock, range);
        const textContent = getCleanText(parentBlock);
        const selectedText = selection.toString();

        if (!selectedText.trim()) {
            activeSelection = null;
            return;
        }

        // Calculate W3C quotes prefix & suffix context
        const blockText = getCleanText(parentBlock);
        const prefixStart = Math.max(0, offsets.start - 20);
        const prefix = blockText.substring(prefixStart, offsets.start);
        const suffixEnd = Math.min(blockText.length, offsets.end + 20);
        const suffix = blockText.substring(offsets.end, suffixEnd);

        activeSelection = {
            type: 'selection',
            blockIdx: getBlockIndex(parentBlock),
            blockHash: hashString(textContent),
            tagName: parentBlock.tagName,
            textQuote: {
                exact: selectedText,
                prefix: prefix,
                suffix: suffix
            },
            textRange: offsets
        };

        // Display popover bubble above selection
        const rect = range.getBoundingClientRect();
        popoverElement.style.left = `${rect.left + window.scrollX + rect.width / 2}px`;
        popoverElement.style.top = `${rect.top + window.scrollY - 36}px`;
        popoverElement.style.display = 'block';
    }

    // ==========================================
    // 6. Paragraph Hover Indicators
    // ==========================================
    function initParagraphIndicators() {
        const blocks = getBlockElements();
        
        blocks.forEach(block => {
            // Apply relative positioning to host container for absolute bubble positioning
            block.style.position = 'relative';
            
            // Check if page doesn't already have an indicator for this paragraph
            let indicator = block.querySelector('.comment-paragraph-indicator');
            if (!indicator) {
                indicator = document.createElement('button');
                indicator.className = 'comment-paragraph-indicator';
                indicator.setAttribute('title', 'Comment on this section');
                indicator.innerHTML = `<svg viewBox="0 0 24 24"><path d="M20 2H4c-1.1 0-1.99.9-1.99 2L2 22l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zM6 9h12v2H6V9zm8 5H6v-2h8v2zm4-6H6V6h12v2z"/></svg>`;
                block.appendChild(indicator);

                indicator.addEventListener('click', (e) => {
                    e.stopPropagation();
                    openSidebar();
                    openNewThreadBox({
                        type: 'paragraph',
                        blockIdx: getBlockIndex(block),
                        blockHash: hashString(getCleanText(block)),
                        tagName: block.tagName
                    });
                });
            }
        });
    }

    // ==========================================
    // 7. Dynamic Sliding Sidebar UI
    // ==========================================
    function initSidebar() {
        sidebarElement = document.createElement('div');
        sidebarElement.className = 'comment-sidebar';
        sidebarElement.innerHTML = `
            <div class="sidebar-header">
                <h3>Comments & Notes</h3>
                <button class="sidebar-close" id="comments-sidebar-close">&times;</button>
            </div>
            
            <div class="sidebar-filter-bar">
                <label class="filter-checkbox-label">
                    <input type="checkbox" id="show-resolved-comments" checked> Show Resolved
                </label>
            </div>

            <div class="sidebar-content" id="comments-sidebar-content">
                <div class="empty-state">No comments yet. Select text or hover over sections to add annotations.</div>
            </div>
        `;
        document.body.appendChild(sidebarElement);

        // Prevent key events from bubbling up to mdbook global keyboard shortcut handlers (like ? or arrow keys)
        // when users are typing inside the sidebar textareas or inputs.
        const stopBubble = (e) => {
            const tag = e.target.tagName;
            if (tag === 'TEXTAREA' || tag === 'INPUT' || tag === 'SELECT') {
                e.stopPropagation();
            }
        };
        sidebarElement.addEventListener('keydown', stopBubble);
        sidebarElement.addEventListener('keypress', stopBubble);
        sidebarElement.addEventListener('keyup', stopBubble);

        // Sidebar close button event listener
        document.getElementById('comments-sidebar-close').addEventListener('click', closeSidebar);
        
        // Filter resolved checkbox change listener
        document.getElementById('show-resolved-comments').addEventListener('change', () => {
            renderThreadsList();
        });

        // Add menu icon button in header menu bar next to theme painter
        const menuLeft = document.querySelector('#mdbook-menu-bar .left-buttons');
        if (menuLeft) {
            // 1. Repo Root Button (Home icon)
            const repoBtn = document.createElement('a');
            repoBtn.className = 'icon-button';
            repoBtn.id = 'repo-root-btn';
            repoBtn.title = 'Go to Repository Root';
            repoBtn.setAttribute('aria-label', 'Go to Repository Root');
            repoBtn.href = '/';
            repoBtn.innerHTML = `<svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor" style="display:inline-block;vertical-align:middle;"><path d="M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z"/></svg>`;
            menuLeft.appendChild(repoBtn);

            // 2. Book Root Button (Book icon)
            let bookRootPath = './';
            try {
                if (typeof path_to_root !== 'undefined') {
                    bookRootPath = path_to_root;
                }
            } catch (e) {}

            // If served in a /book/version structure on production (e.g. /branch/commit_hash/)
            if (window.location.hostname !== 'localhost' && window.location.hostname !== '127.0.0.1') {
                const pathParts = window.location.pathname.split('/').filter(p => p);
                if (pathParts.length >= 2) {
                    bookRootPath = '/' + pathParts[0] + '/';
                }
            }

            const bookBtn = document.createElement('a');
            bookBtn.className = 'icon-button';
            bookBtn.id = 'book-root-btn';
            bookBtn.title = 'Go to Book Root';
            bookBtn.setAttribute('aria-label', 'Go to Book Root');
            bookBtn.href = bookRootPath;
            bookBtn.innerHTML = `<svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor" style="display:inline-block;vertical-align:middle;"><path d="M18 2H6c-1.1 0-2 .9-2 2v16c0 1.1.9 2 2 2h12c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zM6 4h5v8l-2.5-1.5L6 12V4z"/></svg>`;
            menuLeft.appendChild(bookBtn);

            // 3. Comments Sidebar Toggle Button
            const commentsBtn = document.createElement('button');
            commentsBtn.className = 'icon-button';
            commentsBtn.id = 'comments-toggle-btn';
            commentsBtn.title = 'Open Annotation Sidebar';
            commentsBtn.setAttribute('aria-label', 'Open Annotation Sidebar');
            commentsBtn.innerHTML = `<svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor" style="display:inline-block;vertical-align:middle;"><path d="M21.99 4c0-1.1-.89-2-1.99-2H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h14l4 4-.01-18zM18 14H6v-2h12v2zm0-3H6V9h12v2zm0-3H6V6h12v2z"/></svg>`;
            menuLeft.appendChild(commentsBtn);

            commentsBtn.addEventListener('click', (e) => {
                e.stopPropagation();
                toggleSidebar();
            });
        }

        sidebarElement.addEventListener('input', handleAutocompleteInput, true);
        sidebarElement.addEventListener('keydown', handleAutocompleteKeydown, true);
    }

    function toggleSidebar() {
        if (document.documentElement.classList.contains('comments-sidebar-open')) {
            closeSidebar();
        } else {
            openSidebar();
        }
    }

    function openSidebar() {
        document.documentElement.classList.add('comments-sidebar-open');
        renderThreadsList();
    }

    function closeSidebar() {
        document.documentElement.classList.remove('comments-sidebar-open');
        currentOpenThreadId = null;
        closeAutocomplete();
    }

    function focusThread(threadId) {
        currentOpenThreadId = threadId;
        renderThreadsList();
        
        // Scroll to thread card
        setTimeout(() => {
            const card = document.getElementById(`thread-card-${threadId}`);
            if (card) {
                card.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                card.classList.add('focused-card');
                setTimeout(() => card.classList.remove('focused-card'), 1500);
            }
        }, 100);
    }

    function formatTime(isoString) {
        const date = new Date(isoString);
        return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    }

    /**
     * Builds and renders the threads list in the sidebar.
     * Reconstructs comment trees from flat W3C Annotations.
     */
    function renderThreadsList() {
        const content = document.getElementById('comments-sidebar-content');
        if (!content) return;

        // Filter and compile threads
        const showResolved = document.getElementById('show-resolved-comments').checked;
        
        // 1. Separate original annotations (with quote/block target selectors) from reply annotations (targeting annotation ID string)
        const parentAnnos = annotationsList.filter(a => a.target && typeof a.target === 'object');
        const replyAnnos = annotationsList.filter(a => a.target && typeof a.target === 'string');

        // Clean content area
        content.innerHTML = '';

        const visibleParents = parentAnnos.filter(p => showResolved || !p.resolved);

        if (visibleParents.length === 0) {
            content.innerHTML = `<div class="empty-state">No active comments on this page. Select text to highlight and start a conversation.</div>`;
            return;
        }

        // Sort by blockIndex, then selection start offset
        visibleParents.sort((a, b) => {
            const aFrag = a.target.selector.find(s => s.type === 'FragmentSelector');
            const bFrag = b.target.selector.find(s => s.type === 'FragmentSelector');
            const aPos = a.target.selector.find(s => s.type === 'TextPositionSelector');
            const bPos = b.target.selector.find(s => s.type === 'TextPositionSelector');

            const aBlock = aFrag ? parseInt(new URLSearchParams(aFrag.value).get('blockIdx'), 10) : 0;
            const bBlock = bFrag ? parseInt(new URLSearchParams(bFrag.value).get('blockIdx'), 10) : 0;

            if (aBlock !== bBlock) return aBlock - bBlock;
            
            const aStart = aPos ? aPos.start : 0;
            const bStart = bPos ? bPos.start : 0;
            return aStart - bStart;
        });

        visibleParents.forEach(parent => {
            // Find replies targeting this parent
            const threadReplies = replyAnnos.filter(r => r.target === parent.id);
            // Sort replies chronologically
            threadReplies.sort((a, b) => new Date(a.created) - new Date(b.created));

            // Extract selection quote if exists
            const quoteSel = parent.target.selector.find(s => s.type === 'TextQuoteSelector');
            const hasQuote = quoteSel && quoteSel.exact;

            const card = document.createElement('div');
            card.className = `thread-card ${parent.resolved ? 'resolved-card' : ''}`;
            card.id = `thread-card-${parent.id}`;

            let cardHTML = `
                <div class="card-header">
                    <div class="card-author-info">
                        <img src="${parent.creator.avatar || 'https://api.dicebear.com/7.x/adventurer/svg'}" alt="Avatar" class="avatar" crossorigin="anonymous">
                        <div>
                            <span class="author-name">${parent.creator.name}</span>
                            <span class="timestamp">${formatTime(parent.created)}</span>
                        </div>
                    </div>
                    <div class="card-actions">
                        <button class="resolve-btn" data-id="${parent.id}" title="${parent.resolved ? 'Reopen Thread' : 'Resolve Thread'}">
                            ${parent.resolved ? 'Reopen' : 'Resolve'}
                        </button>
                    </div>
                </div>
            `;

            if (hasQuote) {
                cardHTML += `
                    <div class="selection-quote">
                        "${quoteSel.exact}"
                    </div>
                `;
            }

            // Main parent comment body
            const canDeleteParent = true;
            cardHTML += `
                <div class="comment-body">
                    <p class="comment-text">${parent.body.value}</p>
                    ${canDeleteParent ? `<button class="comment-delete-btn" data-id="${parent.id}" title="Delete comment"><svg viewBox="0 0 24 24"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg></button>` : ''}
                </div>
            `;

            // Render nested replies
            if (threadReplies.length > 0) {
                cardHTML += `<div class="replies-container">`;
                threadReplies.forEach(reply => {
                    const canDeleteReply = true;
                    cardHTML += `
                        <div class="reply-card">
                            <div class="reply-header">
                                <img src="${reply.creator.avatar || 'https://api.dicebear.com/7.x/adventurer/svg'}" alt="Avatar" class="avatar-reply" crossorigin="anonymous">
                                <div>
                                    <span class="author-name-reply">${reply.creator.name}</span>
                                    <span class="timestamp-reply">${formatTime(reply.created)}</span>
                                </div>
                            </div>
                            <div class="comment-body-reply">
                                <p class="comment-text-reply">${reply.body.value}</p>
                                ${canDeleteReply ? `<button class="comment-delete-btn-reply" data-id="${reply.id}" title="Delete reply"><svg viewBox="0 0 24 24"><path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/></svg></button>` : ''}
                            </div>
                        </div>
                    `;
                });
                cardHTML += `</div>`;
            }

            // Reply input box
            cardHTML += `
                <div class="card-footer">
                    <textarea class="reply-input" placeholder="Write a reply..." id="reply-input-${parent.id}"></textarea>
                    <button class="reply-submit-btn" data-parent-id="${parent.id}">Reply</button>
                </div>
            `;

            card.innerHTML = cardHTML;
            content.appendChild(card);
        });

        // Attach action event listeners to rendered HTML components
        attachCardListeners();
    }

    function openNewThreadBox(anchor) {
        const content = document.getElementById('comments-sidebar-content');
        if (!content) return;

        // Clear existing temp edit boxes
        const temp = document.getElementById('new-thread-editor-card');
        if (temp) temp.remove();

        const card = document.createElement('div');
        card.className = 'thread-card new-thread-card';
        card.id = 'new-thread-editor-card';

        let innerHTML = `
            <div class="card-header">
                <span class="author-name">New Annotation</span>
                <button class="sidebar-close" id="new-thread-cancel-btn" style="font-size:16px;">Cancel</button>
            </div>
        `;

        if (anchor.type === 'selection') {
            innerHTML += `
                <div class="selection-quote">
                    "${anchor.textQuote.exact}"
                </div>
            `;
        } else {
            innerHTML += `
                <div class="paragraph-context">
                    Annotating entire ${anchor.tagName.toLowerCase()} section
                </div>
            `;
        }

        innerHTML += `
            <div class="card-footer" style="padding-top:10px;">
                <textarea class="reply-input" id="new-thread-text-input" placeholder="Type your comment here..." style="min-height:80px;"></textarea>
                <button class="reply-submit-btn" id="new-thread-submit-btn" style="margin-top:5px;">Post Comment</button>
            </div>
        `;

        card.innerHTML = innerHTML;
        
        // Prepend new thread editor to top of sidebar
        content.insertBefore(card, content.firstChild);
        document.getElementById('new-thread-text-input').focus();

        // Listeners for cancel/submit
        document.getElementById('new-thread-cancel-btn').addEventListener('click', () => {
            card.remove();
        });

        document.getElementById('new-thread-submit-btn').addEventListener('click', async () => {
            const input = document.getElementById('new-thread-text-input');
            const val = input.value.trim();
            if (!val) return;

            // Form standard W3C annotation payload
            const annoPayload = {
                "@context": "http://www.w3.org/ns/anno.jsonld",
                "type": "Annotation",
                "body": {
                    "type": "TextualBody",
                    "value": val,
                    "format": "text/markdown"
                },
                "target": {
                    "source": CURRENT_PAGE,
                    "selector": []
                },
                "resolved": false
            };

            // Inject paragraph fragment selector
            annoPayload.target.selector.push({
                "type": "FragmentSelector",
                "value": `blockIdx=${anchor.blockIdx}&blockHash=${anchor.blockHash}&tagName=${anchor.tagName}`
            });

            if (anchor.type === 'selection') {
                // Inject selection quote selector
                annoPayload.target.selector.push({
                    "type": "TextQuoteSelector",
                    "exact": anchor.textQuote.exact,
                    "prefix": anchor.textQuote.prefix,
                    "suffix": anchor.textQuote.suffix
                });
                // Inject selection text position offset selector
                annoPayload.target.selector.push({
                    "type": "TextPositionSelector",
                    "start": anchor.textRange.start,
                    "end": anchor.textRange.end
                });
            }

            const saved = await CommentsAPI.saveAnnotation(annoPayload);
            if (saved) {
                annotationsList.push(saved);
                card.remove();
                renderThreadsList();
                applyPageHighlights();
            }
        });
    }

    function attachCardListeners() {
        // Resolve buttons
        document.querySelectorAll('.resolve-btn').forEach(btn => {
            btn.addEventListener('click', async (e) => {
                const id = e.target.dataset.id;
                const anno = annotationsList.find(a => a.id === id);
                if (anno) {
                    const updated = await CommentsAPI.resolveThread(id, !anno.resolved);
                    if (updated) {
                        anno.resolved = updated.resolved;
                        renderThreadsList();
                        applyPageHighlights();
                    }
                }
            });
        });

        // Submit replies
        document.querySelectorAll('.reply-submit-btn').forEach(btn => {
            btn.addEventListener('click', async (e) => {
                const parentId = e.target.dataset.parentId;
                const input = document.getElementById(`reply-input-${parentId}`);
                const val = input.value.trim();
                if (!val) return;

                const replyPayload = {
                    "@context": "http://www.w3.org/ns/anno.jsonld",
                    "type": "Annotation",
                    "body": {
                        "type": "TextualBody",
                        "value": val,
                        "format": "text/markdown"
                    },
                    "target": parentId,
                    "motivation": "replying"
                };

                const savedReply = await CommentsAPI.saveAnnotation(replyPayload);
                if (savedReply) {
                    annotationsList.push(savedReply);
                    renderThreadsList();
                }
            });
        });

        // Delete parent comment / thread
        document.querySelectorAll('.comment-delete-btn').forEach(btn => {
            btn.addEventListener('click', async (e) => {
                const id = btn.dataset.id;
                if (confirm('Are you sure you want to delete this comment thread? This will cascade delete all replies.')) {
                    const success = await CommentsAPI.deleteAnnotation(id);
                    if (success) {
                        // Filter parent and all its replies targeting it
                        annotationsList = annotationsList.filter(a => a.id !== id && a.target !== id);
                        renderThreadsList();
                        applyPageHighlights();
                    }
                }
            });
        });

        // Delete reply comments
        document.querySelectorAll('.comment-delete-btn-reply').forEach(btn => {
            btn.addEventListener('click', async (e) => {
                const id = btn.dataset.id;
                if (confirm('Are you sure you want to delete this reply?')) {
                    const success = await CommentsAPI.deleteAnnotation(id);
                    if (success) {
                        annotationsList = annotationsList.filter(a => a.id !== id);
                        renderThreadsList();
                    }
                }
            });
        });

        // Click on thread card to scroll target element into view
        document.querySelectorAll('.thread-card:not(.new-thread-card)').forEach(card => {
            card.addEventListener('click', (e) => {
                if (e.target.closest('a, button, input, textarea')) {
                    return;
                }
                const threadId = card.id.replace('thread-card-', '');
                const targetElement = document.querySelector(`[data-thread-id="${threadId}"]`);
                if (targetElement) {
                    targetElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
                    
                    targetElement.classList.remove('comment-highlight-flash');
                    void targetElement.offsetWidth; // trigger reflow
                    targetElement.classList.add('comment-highlight-flash');
                    setTimeout(() => {
                        targetElement.classList.remove('comment-highlight-flash');
                    }, 2000);
                }
            });
        });
    }

    // ==========================================
    // 7. Mentions Autocomplete Engine
    // ==========================================
    async function fetchCurrentUser() {
        try {
            const res = await fetch('/_/api/v1/auth/me');
            if (res.ok) {
                const data = await res.json();
                if (data.authenticated && data.user) {
                    currentUser = data.user;
                }
            }
        } catch (e) {
            console.error('[Comments] Failed to fetch current user:', e);
        }
    }

    function getAutocompleteCandidates() {
        const candidates = new Map();
        
        // 1. Add current user
        if (currentUser && currentUser.email) {
            candidates.set(currentUser.email.toLowerCase(), {
                name: currentUser.name,
                email: currentUser.email
            });
        }

        // 2. Add page commenters
        annotationsList.forEach(anno => {
            if (anno.creator && anno.creator.email) {
                const email = anno.creator.email.toLowerCase();
                if (!candidates.has(email)) {
                    candidates.set(email, {
                        name: anno.creator.name,
                        email: anno.creator.email
                    });
                }
            }
        });

        return Array.from(candidates.values());
    }

    function getActiveMentionQuery(textarea) {
        const val = textarea.value;
        const start = textarea.selectionStart;
        
        // Find the start of the current word by looking back
        let wordStart = start;
        while (wordStart > 0 && !/\s/.test(val[wordStart - 1])) {
            wordStart--;
        }
        
        const word = val.substring(wordStart, start);
        if (word.startsWith('@')) {
            return {
                query: word.substring(1),
                startIdx: wordStart,
                endIdx: start
            };
        }
        return null;
    }

    function handleAutocompleteInput(e) {
        const textarea = e.target;
        if (!textarea.classList.contains('reply-input') && textarea.id !== 'new-thread-text-input') {
            return;
        }

        activeAutocompleteTextarea = textarea;
        const mentionInfo = getActiveMentionQuery(textarea);
        activeMentionInfo = mentionInfo;

        if (!mentionInfo) {
            closeAutocomplete();
            return;
        }

        const query = mentionInfo.query.toLowerCase();
        const candidates = getAutocompleteCandidates();
        const matches = candidates.filter(c => 
            (c.name && c.name.toLowerCase().includes(query)) || 
            (c.email && c.email.toLowerCase().includes(query))
        );

        if (matches.length === 0) {
            closeAutocomplete();
            return;
        }

        renderAutocompleteDropdown(textarea, matches);
    }

    function renderAutocompleteDropdown(textarea, matches) {
        if (!autocompleteDropdown) {
            autocompleteDropdown = document.createElement('ul');
            autocompleteDropdown.className = 'comments-autocomplete-dropdown';
            document.body.appendChild(autocompleteDropdown);
        }

        autocompleteDropdown.innerHTML = '';
        activeDropdownIndex = Math.min(activeDropdownIndex, matches.length - 1);
        activeDropdownIndex = Math.max(0, activeDropdownIndex);

        matches.forEach((candidate, idx) => {
            const li = document.createElement('li');
            li.className = 'comments-autocomplete-item';
            if (idx === activeDropdownIndex) {
                li.classList.add('active');
            }

            const nameSpan = document.createElement('span');
            nameSpan.className = 'autocomplete-name';
            nameSpan.textContent = candidate.name || 'Anonymous';

            const emailSpan = document.createElement('span');
            emailSpan.className = 'autocomplete-email';
            emailSpan.textContent = candidate.email;

            li.appendChild(nameSpan);
            li.appendChild(emailSpan);

            li.addEventListener('mousedown', (e) => {
                // Use mousedown instead of click to fire before textarea blur
                e.preventDefault();
                e.stopPropagation();
                selectAutocompleteCandidate(candidate);
            });

            autocompleteDropdown.appendChild(li);
        });

        const rect = textarea.getBoundingClientRect();
        autocompleteDropdown.style.top = `${rect.bottom}px`;
        autocompleteDropdown.style.left = `${rect.left}px`;
        autocompleteDropdown.style.width = `${rect.width}px`;
        autocompleteDropdown.style.display = 'block';

        autocompleteDropdown.dataset.matches = JSON.stringify(matches);
    }

    function handleAutocompleteKeydown(e) {
        if (!autocompleteDropdown || autocompleteDropdown.style.display === 'none') {
            return;
        }

        const matches = JSON.parse(autocompleteDropdown.dataset.matches || '[]');
        if (matches.length === 0) return;

        if (e.key === 'ArrowDown') {
            e.preventDefault();
            e.stopPropagation();
            activeDropdownIndex = (activeDropdownIndex + 1) % matches.length;
            updateActiveAutocompleteItem();
        } else if (e.key === 'ArrowUp') {
            e.preventDefault();
            e.stopPropagation();
            activeDropdownIndex = (activeDropdownIndex - 1 + matches.length) % matches.length;
            updateActiveAutocompleteItem();
        } else if (e.key === 'Enter' || e.key === 'Tab') {
            e.preventDefault();
            e.stopPropagation();
            selectAutocompleteCandidate(matches[activeDropdownIndex]);
        } else if (e.key === 'Escape') {
            e.preventDefault();
            e.stopPropagation();
            closeAutocomplete();
        }
    }

    function updateActiveAutocompleteItem() {
        if (!autocompleteDropdown) return;
        const items = autocompleteDropdown.querySelectorAll('.comments-autocomplete-item');
        items.forEach((item, idx) => {
            if (idx === activeDropdownIndex) {
                item.classList.add('active');
                item.scrollIntoView({ block: 'nearest' });
            } else {
                item.classList.remove('active');
            }
        });
    }

    function selectAutocompleteCandidate(candidate) {
        if (!activeAutocompleteTextarea || !activeMentionInfo) return;
        
        const textarea = activeAutocompleteTextarea;
        const val = textarea.value;
        const start = activeMentionInfo.startIdx;
        const end = activeMentionInfo.endIdx;
        const insertedText = `@${candidate.email} `;

        textarea.value = val.substring(0, start) + insertedText + val.substring(end);
        
        const newCursorPos = start + insertedText.length;
        textarea.setSelectionRange(newCursorPos, newCursorPos);
        
        closeAutocomplete();
        textarea.focus();
        textarea.dispatchEvent(new Event('input', { bubbles: true }));
    }

    function closeAutocomplete() {
        if (autocompleteDropdown) {
            autocompleteDropdown.style.display = 'none';
        }
        activeDropdownIndex = 0;
        activeMentionInfo = null;
    }

    // Close autocomplete on click outside, window resize, or escape
    document.addEventListener('click', (e) => {
        if (autocompleteDropdown && !autocompleteDropdown.contains(e.target) && e.target !== activeAutocompleteTextarea) {
            closeAutocomplete();
        }
    });

    window.addEventListener('resize', closeAutocomplete);

    // ==========================================
    // 8. Bootstrap initialization
    // ==========================================
    async function boot() {
        // Fetch current user and page comments
        await fetchCurrentUser();
        annotationsList = await CommentsAPI.getAnnotations();

        // Setup UI layouts
        initSidebar();
        initSelectionTracker();
        initHighlightClickListener();
        initParagraphIndicators();

        // Close autocomplete when scrolling sidebar content
        const sidebarContent = document.getElementById('comments-sidebar-content');
        if (sidebarContent) {
            sidebarContent.addEventListener('scroll', closeAutocomplete);
        }

        // Apply range highlights
        applyPageHighlights();

        // Open sidebar automatically if there are unresolved parent comments
        const hasUnresolved = annotationsList.some(a => a.target && typeof a.target === 'object' && !a.resolved);
        if (hasUnresolved) {
            openSidebar();
        }

        console.log('[Comments] Custom Comments Integration booted successfully.');
    }

    // Run on DOM load
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', boot);
    } else {
        boot();
    }

})();
